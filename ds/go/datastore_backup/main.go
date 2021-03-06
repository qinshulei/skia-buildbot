// Trigger backups of Cloud Datastore entities to Cloud Storage using the
// datastore v1beta1 API.
//
// See http://go/datastore-backup-example for an example in the APIs Explorer.
package main

import (
	"flag"
	"time"

	"cloud.google.com/go/datastore"
	"go.skia.org/infra/ds/go/backup"
	"go.skia.org/infra/go/auth"
	"go.skia.org/infra/go/common"
	"go.skia.org/infra/go/httputils"
	"go.skia.org/infra/go/sklog"
)

// flags
var (
	promPort = flag.String("prom_port", ":20000", "Metrics service address (e.g., ':10110')")
)

const (
	BUCKET  = "skia-backups"
	PROJECT = "google.com:skia-buildbots"
)

func main() {
	common.InitWithMust(
		"datastore_backup",
		common.PrometheusOpt(promPort),
		common.CloudLoggingOpt(),
	)

	ts, err := auth.NewJWTServiceAccountTokenSource("", "", datastore.ScopeDatastore)
	if err != nil {
		sklog.Fatalf("Failed to auth: %s", err)
	}
	// backup package handles retries and specifically handles "resource exhausted" HTTP status code.
	client := httputils.DefaultClientConfig().WithTokenSource(ts).WithoutRetries().Client()
	if err := backup.Step(client, PROJECT, BUCKET); err != nil {
		sklog.Errorf("Failed to do first backup step: %s", err)
	}
	for _ = range time.Tick(24 * time.Hour) {
		if err := backup.Step(client, PROJECT, BUCKET); err != nil {
			sklog.Errorf("Failed to backup: %s", err)
		}
	}
}
