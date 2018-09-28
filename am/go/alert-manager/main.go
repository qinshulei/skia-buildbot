package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/99designs/goodies/http/secure_headers/csp"
	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	"github.com/unrolled/secure"
	"go.skia.org/infra/am/go/incident"
	"go.skia.org/infra/am/go/note"
	"go.skia.org/infra/am/go/silence"
	"go.skia.org/infra/go/alerts"
	"go.skia.org/infra/go/allowed"
	"go.skia.org/infra/go/auditlog"
	"go.skia.org/infra/go/auth"
	"go.skia.org/infra/go/common"
	"go.skia.org/infra/go/ds"
	"go.skia.org/infra/go/httputils"
	"go.skia.org/infra/go/login"
	"go.skia.org/infra/go/sklog"
	"go.skia.org/infra/go/util"
	"google.golang.org/api/option"
)

// flags
var (
	assignGroup        = flag.String("assign_group", "google/skia-root@google.com", "The chrome infra auth group to use for users incidents can be assigned to.")
	authGroup          = flag.String("auth_group", "google/skia-staff@google.com", "The chrome infra auth group to use for restricting access.")
	chromeInfraAuthJWT = flag.String("chrome_infra_auth_jwt", "/var/secrets/skia-public-auth/key.json", "The JWT key for the service account that has access to chrome infra auth.")
	local              = flag.Bool("local", false, "Running locally if true. As opposed to in production.")
	namespace          = flag.String("namespace", "", "The Cloud Datastore namespace, such as 'perf'.")
	port               = flag.String("port", ":8000", "HTTP service address (e.g., ':8000')")
	internalPort       = flag.String("internal_port", ":9000", "HTTP internal service address (e.g., ':9000') for unauthenticated in-cluster requests.")
	project            = flag.String("project", "skia-public", "The Google Cloud project name.")
	promPort           = flag.String("prom_port", ":20000", "Metrics service address (e.g., ':10110')")
	resourcesDir       = flag.String("resources_dir", "", "The directory to find templates, JS, and CSS files. If blank the current directory will be used.")
)

const (
	// EXPIRE_DURATION is the time to wait before expiring an incident.
	EXPIRE_DURATION = 2 * time.Minute

	APP_NAME = "alert-manager"
)

// Server is the state of the server.
type Server struct {
	incidentStore *incident.Store
	silenceStore  *silence.Store
	templates     *template.Template
	salt          []byte        // Salt for csrf cookies.
	allow         allowed.Allow // Who is allowed to use the site.
	assign        allowed.Allow // A list of people that incidents can be assigned to.
}

func New() (*Server, error) {
	if *resourcesDir == "" {
		_, filename, _, _ := runtime.Caller(0)
		*resourcesDir = filepath.Join(filepath.Dir(filename), "../../dist")
	}

	// Setup the salt.
	salt := []byte("32-byte-long-auth-key")
	if !*local {
		var err error
		salt, err = ioutil.ReadFile("/var/skia/salt.txt")
		if err != nil {
			return nil, err
		}
	}

	var allow allowed.Allow
	var assign allowed.Allow
	if !*local {
		ts, err := auth.NewJWTServiceAccountTokenSource("", *chromeInfraAuthJWT, auth.SCOPE_USERINFO_EMAIL)
		if err != nil {
			return nil, err
		}
		client := httputils.DefaultClientConfig().WithTokenSource(ts).With2xxOnly().Client()
		allow, err = allowed.NewAllowedFromChromeInfraAuth(client, *authGroup)
		if err != nil {
			return nil, err
		}
		assign, err = allowed.NewAllowedFromChromeInfraAuth(client, *assignGroup)
		if err != nil {
			return nil, err
		}
	} else {
		allow = allowed.NewAllowedFromList([]string{"fred@example.org", "barney@example.org", "wilma@example.org"})
		assign = allowed.NewAllowedFromList([]string{"betty@example.org", "fred@example.org", "barney@example.org", "wilma@example.org"})
	}

	login.InitWithAllow(*port, *local, nil, nil, allow)

	ctx := context.Background()
	ts, err := auth.NewDefaultTokenSource(*local, pubsub.ScopePubSub, "https://www.googleapis.com/auth/datastore")
	if err != nil {
		return nil, err
	}

	if *namespace == "" {
		return nil, fmt.Errorf("The --namespace flag is required. See infra/DATASTORE.md for format details.\n")
	}
	if !*local && !util.In(*namespace, []string{ds.ALERT_MANAGER_NS}) {
		return nil, fmt.Errorf("When running in prod the datastore namespace must be a known value.")
	}
	if err := ds.InitWithOpt(*project, *namespace, option.WithTokenSource(ts)); err != nil {
		return nil, fmt.Errorf("Failed to init Cloud Datastore: %s", err)
	}

	client, err := pubsub.NewClient(ctx, *project, option.WithTokenSource(ts))
	if err != nil {
		return nil, err
	}
	topic := client.Topic(alerts.TOPIC)
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	// When running in production we have every instance use the same topic name so that
	// they load-balance pulling items from the topic.
	subName := fmt.Sprintf("%s-%s", alerts.TOPIC, "prod")
	if *local {
		// When running locally create a new topic for every host.
		subName = fmt.Sprintf("%s-%s", alerts.TOPIC, hostname)
	}
	sub := client.Subscription(subName)
	ok, err := sub.Exists(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed checking subscription existence: %s", err)
	}
	if !ok {
		sub, err = client.CreateSubscription(ctx, subName, pubsub.SubscriptionConfig{
			Topic: topic,
		})
		if err != nil {
			return nil, fmt.Errorf("Failed creating subscription: %s", err)
		}
	}

	srv := &Server{
		salt:          salt,
		incidentStore: incident.NewStore(ds.DS, []string{"kubernetes_pod_name", "instance", "pod_template_hash"}),
		silenceStore:  silence.NewStore(ds.DS),
		allow:         allow,
		assign:        assign,
	}
	srv.loadTemplates()

	// Process all incoming PubSub requests.
	go func() {
		for {
			err := sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
				msg.Ack()
				var m map[string]string
				if err := json.Unmarshal(msg.Data, &m); err != nil {
					sklog.Error(err)
					return
				}
				if m[alerts.TYPE] == alerts.TYPE_HEALTHZ {
					sklog.Infof("healthz received: %q", m[alerts.LOCATION])
				} else {
					if _, err := srv.incidentStore.AlertArrival(m); err != nil {
						sklog.Errorf("Error processing alert: %s", err)
					}
				}
			})
			if err != nil {
				sklog.Errorf("Failed receiving pubsub message: %s", err)
			}
		}
	}()

	// This is really just a backstop in case we miss a resolved event for the incident.
	go func() {
		for _ = range time.Tick(1 * time.Minute) {
			ins, err := srv.incidentStore.GetAll()
			if err != nil {
				sklog.Errorf("Failed to load incidents: %s", err)
				continue
			}
			now := time.Now()
			for _, in := range ins {
				// If it was last updated too long ago then it should be archived.
				if time.Unix(in.LastSeen, 0).Add(EXPIRE_DURATION).Before(now) {
					if _, err := srv.incidentStore.Archive(in.Key); err != nil {
						sklog.Errorf("Failed to archive incident: %s", err)
					}
				}
			}
		}
	}()

	return srv, nil
}

func (srv *Server) loadTemplates() {
	srv.templates = template.Must(template.New("").Delims("{%", "%}").ParseFiles(
		filepath.Join(*resourcesDir, "index.html"),
	))
}

func (srv *Server) mainHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	if *local {
		srv.loadTemplates()
	}
	if err := srv.templates.ExecuteTemplate(w, "index.html", map[string]string{
		// base64 encode the csrf to avoid golang templating escaping.
		"csrf": base64.StdEncoding.EncodeToString([]byte(csrf.Token(r))),
	}); err != nil {
		sklog.Errorf("Failed to expand template: %s", err)
	}
}

type AddNoteRequest struct {
	Text string `json:"text"`
	Key  string `json:"key"`
}

// user returns the currently logged in user, or a placeholder if running locally.
func (srv *Server) user(r *http.Request) string {
	user := "barney@example.org"
	if !*local {
		user = login.LoggedInAs(r)
	}
	return user
}

func (srv *Server) addNoteHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req AddNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.ReportError(w, r, err, "Failed to decode add note request.")
		return
	}
	auditlog.Log(r, "add-note", req)

	note := note.Note{
		Text:   req.Text,
		TS:     time.Now().Unix(),
		Author: srv.user(r),
	}
	in, err := srv.incidentStore.AddNote(req.Key, note)
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to add note.")
		return
	}
	if err := json.NewEncoder(w).Encode(in); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

func (srv *Server) addSilenceNoteHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req AddNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.ReportError(w, r, err, "Failed to decode add note request.")
		return
	}
	auditlog.Log(r, "add-silence-note", req)

	note := note.Note{
		Text:   req.Text,
		TS:     time.Now().Unix(),
		Author: srv.user(r),
	}
	in, err := srv.silenceStore.AddNote(req.Key, note)
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to add note.")
		return
	}
	if err := json.NewEncoder(w).Encode(in); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

type DelNoteRequest struct {
	Index int    `json:"index"`
	Key   string `json:"key"`
}

func (srv *Server) delNoteHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req DelNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.ReportError(w, r, err, "Failed to decode add note request.")
		return
	}
	auditlog.Log(r, "del-note", req)
	in, err := srv.incidentStore.DeleteNote(req.Key, req.Index)
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to add note.")
		return
	}
	if err := json.NewEncoder(w).Encode(in); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

func (srv *Server) delSilenceNoteHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req DelNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.ReportError(w, r, err, "Failed to decode add note request.")
		return
	}
	auditlog.Log(r, "del-silence-note", req)
	in, err := srv.silenceStore.DeleteNote(req.Key, req.Index)
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to add note.")
		return
	}
	if err := json.NewEncoder(w).Encode(in); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

type TakeRequest struct {
	Key string `json:"key"`
}

func (srv *Server) takeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req TakeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.ReportError(w, r, err, "Failed to decode take request.")
		return
	}
	auditlog.Log(r, "take", req)

	in, err := srv.incidentStore.Assign(req.Key, srv.user(r))
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to assign.")
		return
	}
	if err := json.NewEncoder(w).Encode(in); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

type StatsRequest struct {
	Range string `json:"range"`
}

type Stat struct {
	Num      int               `json:"num"`
	Incident incident.Incident `json:"incident"`
}

type StatsResponse []*Stat

type StatsResponseSlice StatsResponse

func (p StatsResponseSlice) Len() int           { return len(p) }
func (p StatsResponseSlice) Less(i, j int) bool { return p[i].Num > p[j].Num }
func (p StatsResponseSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func (srv *Server) statsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req StatsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.ReportError(w, r, err, "Failed to decode stats request.")
		return
	}
	ins, err := srv.incidentStore.GetRecentlyResolvedInRange(req.Range)
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to query for Incidents.")
	}
	count := map[string]*Stat{}
	for _, in := range ins {
		if stat, ok := count[in.ID]; !ok {
			count[in.ID] = &Stat{
				Num:      1,
				Incident: in,
			}
		} else {
			stat.Num += 1
		}
	}
	ret := StatsResponse{}
	for _, v := range count {
		ret = append(ret, v)
	}
	sort.Sort(StatsResponseSlice(ret))
	if err := json.NewEncoder(w).Encode(ret); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

type IncidentsInRangeRequest struct {
	Range    string            `json:"range"`
	Incident incident.Incident `json:"incident"`
}

func (srv *Server) incidentsInRangeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req IncidentsInRangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.ReportError(w, r, err, "Failed to decode incident range request.")
		return
	}
	ret, err := srv.incidentStore.GetRecentlyResolvedInRangeWithID(req.Range, req.Incident.ID)
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to query for incidents.")
	}
	if err := json.NewEncoder(w).Encode(ret); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

type AssignRequest struct {
	Key   string `json:"key"`
	Email string `json:"email"`
}

func (srv *Server) assignHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req AssignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.ReportError(w, r, err, "Failed to decode take request.")
		return
	}
	auditlog.Log(r, "assign", req)
	in, err := srv.incidentStore.Assign(req.Key, req.Email)
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to assign.")
		return
	}
	if err := json.NewEncoder(w).Encode(in); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

func (srv *Server) emailsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	emails := srv.assign.Emails()
	sort.Strings(emails)
	if err := json.NewEncoder(w).Encode(&emails); err != nil {
		sklog.Errorf("Failed to encode emails: %s", err)
	}
}

func (srv *Server) silencesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	silences, err := srv.silenceStore.GetAll()
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to load recents.")
		return
	}
	if silences == nil {
		silences = []silence.Silence{}
	}
	recents, err := srv.silenceStore.GetRecentlyArchived()
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to load recents.")
		return
	}
	silences = append(silences, recents...)
	if err := json.NewEncoder(w).Encode(silences); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

func (srv *Server) incidentHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ins, err := srv.incidentStore.GetAll()
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to load incidents.")
		return
	}
	recents, err := srv.incidentStore.GetRecentlyResolved()
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to load recents.")
		return
	}
	ins = append(ins, recents...)
	if err := json.NewEncoder(w).Encode(ins); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

func (srv *Server) recentIncidentsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := r.FormValue("id")
	key := r.FormValue("key")
	ins, err := srv.incidentStore.GetRecentlyResolvedForID(id, key)
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to load incidents.")
		return
	}
	if err := json.NewEncoder(w).Encode(ins); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

func (srv *Server) saveSilenceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req silence.Silence
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.ReportError(w, r, err, "Failed to decode silence creation request.")
		return
	}
	auditlog.Log(r, "create-silence", req)
	silence, err := srv.silenceStore.Put(&req)
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to create silence.")
		return
	}
	if err := json.NewEncoder(w).Encode(silence); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

func (srv *Server) archiveSilenceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req silence.Silence
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.ReportError(w, r, err, "Failed to decode silence creation request.")
		return
	}
	auditlog.Log(r, "archive-silence", req)
	silence, err := srv.silenceStore.Archive(req.Key)
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to archive silence.")
		return
	}
	if err := json.NewEncoder(w).Encode(silence); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

func (srv *Server) reactivateSilenceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req silence.Silence
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.ReportError(w, r, err, "Failed to decode silence creation request.")
		return
	}
	auditlog.Log(r, "reactivate-silence", req)
	silence, err := srv.silenceStore.Reactivate(req.Key, srv.user(r))
	if err != nil {
		httputils.ReportError(w, r, err, "Failed to archive silence.")
		return
	}
	if err := json.NewEncoder(w).Encode(silence); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

// newSilenceHandler creates and returns a new Silence pre-populated with good defaults.
func (srv *Server) newSilenceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s := silence.New(srv.user(r))
	if err := json.NewEncoder(w).Encode(s); err != nil {
		sklog.Errorf("Failed to send response: %s", err)
	}
}

func (srv *Server) applySecurityWrappers(h http.Handler) http.Handler {
	// Configure Content Security Policy (CSP).
	cspOpts := csp.Opts{
		DefaultSrc: []string{csp.SourceNone},
		ConnectSrc: []string{"https://skia.org", "https://skia-tree-status.appspot.com", csp.SourceSelf},
		ImgSrc:     []string{csp.SourceSelf},
		StyleSrc:   []string{csp.SourceSelf},
		ScriptSrc:  []string{csp.SourceSelf},
	}

	if *local {
		// webpack uses eval() in development mode, so allow unsafe-eval when local.
		cspOpts.ScriptSrc = append(cspOpts.ScriptSrc, "'unsafe-eval'")
	}

	// Apply CSP and other security minded headers.
	secureMiddleware := secure.New(secure.Options{
		AllowedHosts:          []string{"am.skia.org"},
		HostsProxyHeaders:     []string{"X-Forwarded-Host"},
		SSLRedirect:           true,
		SSLProxyHeaders:       map[string]string{"X-Forwarded-Proto": "https"},
		STSSeconds:            60 * 60 * 24 * 365,
		STSIncludeSubdomains:  true,
		ContentSecurityPolicy: cspOpts.Header(),
		IsDevelopment:         *local,
	})

	h = secureMiddleware.Handler(h)
	h = csrf.Protect(srv.salt, csrf.Secure(!*local))(h)
	return h
}

func main() {
	common.InitWithMust(
		APP_NAME,
		common.PrometheusOpt(promPort),
		common.MetricsLoggingOpt(),
	)

	srv, err := New()
	if err != nil {
		sklog.Fatalf("Failed to create Server: %s", err)
	}

	// Internal endpoints that are only accessible from within the cluster.
	unprotected := mux.NewRouter()
	unprotected.HandleFunc("/_/incidents", srv.incidentHandler).Methods("GET")
	go func() {
		sklog.Fatal(http.ListenAndServe(*internalPort, unprotected))
	}()

	r := mux.NewRouter()
	r.HandleFunc("/", srv.mainHandler)
	// GETs
	r.HandleFunc("/_/emails", srv.emailsHandler).Methods("GET")
	r.HandleFunc("/_/incidents", srv.incidentHandler).Methods("GET")
	r.HandleFunc("/_/new_silence", srv.newSilenceHandler).Methods("GET")
	r.HandleFunc("/_/recent_incidents", srv.recentIncidentsHandler).Methods("GET")
	r.HandleFunc("/_/silences", srv.silencesHandler).Methods("GET")
	r.HandleFunc("/loginstatus/", login.StatusHandler).Methods("GET")

	// POSTs
	r.HandleFunc("/_/add_note", srv.addNoteHandler).Methods("POST")
	r.HandleFunc("/_/add_silence_note", srv.addSilenceNoteHandler).Methods("POST")
	r.HandleFunc("/_/archive_silence", srv.archiveSilenceHandler).Methods("POST")
	r.HandleFunc("/_/assign", srv.assignHandler).Methods("POST")
	r.HandleFunc("/_/del_note", srv.delNoteHandler).Methods("POST")
	r.HandleFunc("/_/del_silence_note", srv.delSilenceNoteHandler).Methods("POST")
	r.HandleFunc("/_/reactivate_silence", srv.reactivateSilenceHandler).Methods("POST")
	r.HandleFunc("/_/save_silence", srv.saveSilenceHandler).Methods("POST")
	r.HandleFunc("/_/take", srv.takeHandler).Methods("POST")
	r.HandleFunc("/_/stats", srv.statsHandler).Methods("POST")
	r.HandleFunc("/_/incidents_in_range", srv.incidentsInRangeHandler).Methods("POST")

	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.HandlerFunc(httputils.MakeResourceHandler(*resourcesDir))))

	h := srv.applySecurityWrappers(r)
	if !*local {
		h = httputils.LoggingGzipRequestResponse(h)
		h = login.RestrictViewer(h)
		h = login.ForceAuth(h, login.DEFAULT_REDIRECT_URL)
		h = httputils.HealthzAndHTTPS(h)
	}
	http.Handle("/", h)
	sklog.Infoln("Ready to serve.")
	sklog.Fatal(http.ListenAndServe(*port, nil))
}
