/**
 * common.js is a set of common functions used across all of skiaperf.
 *
 * Everything is scoped to 'sk' except $$ and $$$ which are global since they
 * are used so often.
 *
 */

/**
 * $$ returns a real JS array of DOM elements that match the CSS query selector.
 *
 * A shortcut for jQuery-like $ behavior.
 **/
function $$(query, ele) {
  if (!ele) {
    ele = document;
  }
  return Array.prototype.map.call(ele.querySelectorAll(query), function(e) { return e; });
}


/**
 * $$$ returns the DOM element that match the CSS query selector.
 *
 * A shortcut for document.querySelector.
 **/
function $$$(query, ele) {
  if (!ele) {
    ele = document;
  }
  return ele.querySelector(query);
}


this.sk = this.sk || function() {
  "use strict";

  var sk = {};


  /**
   * clearChildren removes all children of the passed in node.
   */
  sk.clearChildren = function(ele) {
    while (ele.firstChild) {
      ele.removeChild(ele.firstChild);
    }
  }

  /**
   * findParent returns either 'ele' or a parent of 'ele' that has the nodeName of 'nodeName'.
   *
   * Note that nodeName is all caps, i.e. "DIV" or "PAPER-BUTTON".
   *
   * The return value is null if no containing element has that node name.
   */
  sk.findParent = function(ele, nodeName) {
    while (ele != null) {
      if (ele.nodeName == nodeName) {
        return ele;
      }
      ele = ele.parentElement;
    }
    return null;
  }

  /**
   * errorMessage dispatches an event with the error message in it.
   *
   * See <error-toast-sk> for an element that listens for such events
   * and displays the error messages.
   *
   */
  sk.errorMessage = function(message) {
    var detail = {
      message: message,
    }
    document.dispatchEvent(new CustomEvent('error-sk', {detail: detail, bubbles: true}));
  }

  /**
   * Importer simplifies importing HTML Templates from HTML Imports.
   *
   * Just instantiate an instance in the HTML Import:
   *
   *    importer = new sk.Importer();
   *
   * Then import templates via their id:
   *
   *    var node = importer.import('#foo');
   */
  sk.Importer = function() {
    if ('currentScript' in document) {
      this.importDoc_ = document.currentScript.ownerDocument;
    } else {
      this.importDoc_ = document._currentScript.ownerDocument;
    }
  }

  sk.Importer.prototype.import = function(id) {
    return document.importNode($$$(id, this.importDoc_).content, true);
  }

  // elePos returns the position of the top left corner of given element in
  // client coordinates.
  //
  // Returns an object of the form:
  // {
  //   x: NNN,
  //   y: MMM,
  // }
  sk.elePos = function(ele) {
    var bounds = ele.getBoundingClientRect();
    return {x: bounds.left, y: bounds.top};
  }

  // Returns a Promise that uses XMLHttpRequest to make a request with the given
  // method to the given URL with the given headers and body.
  sk.request = function(method, url, body, headers) {
    // Return a new promise.
    return new Promise(function(resolve, reject) {
      // Do the usual XHR stuff
      var req = new XMLHttpRequest();
      req.open(method, url);
      if (headers) {
        for (var k in headers) {
          req.setRequestHeader(k, headers[k]);
        }
      }

      req.onload = function() {
        // This is called even on 404 etc
        // so check the status
        if (req.status == 200) {
          // Resolve the promise with the response text
          resolve(req.response);
        } else {
          // Otherwise reject with the status text
          // which will hopefully be a meaningful error
          reject(req.response);
        }
      };

      // Handle network errors
      req.onerror = function() {
        reject(Error("Network Error"));
      };

      // Make the request
      req.send(body);
    });
  }

  // Returns a Promise that uses XMLHttpRequest to make a request to the given URL.
  sk.get = function(url) {
    return sk.request('GET', url);
  }


  // Returns a Promise that uses XMLHttpRequest to make a POST request to the
  // given URL with the given JSON body.
  sk.post = function(url, body) {
    return sk.request('POST', url, body, {"Content-Type": "application/json"});
  }

  // Returns a Promise that uses XMLHttpRequest to make a DELETE request to the
  // given URL.
  sk.delete = function(url) {
    return sk.request('DELETE', url);
  }

  // A Promise that resolves when DOMContentLoaded has fired.
  sk.DomReady = new Promise(function(resolve, reject) {
      if (document.readyState != 'loading') {
        // If readyState is already past loading then
        // DOMContentLoaded has already fired, so just resolve.
        resolve();
      } else {
        document.addEventListener('DOMContentLoaded', resolve);
      }
    });

  // A Promise that resolves when Polymer has fired polymer-ready.
  sk.WebComponentsReady = new Promise(function(resolve, reject) {
    window.addEventListener('polymer-ready', resolve);
  });

  // _Mailbox is an object that allows distributing, possibly in a time
  // delayed manner, values to subscribers to mailbox names.
  //
  // For example, a series of large objects may need to be distributed across
  // a DOM tree in a way that doesn't easily fit with normal data binding.
  // Instead each element can subscribe to a mailbox name where the data will
  // be placed, and receive a callback when the data is updated. Note that
  // upon first subscribing to a mailbox the callback will be triggered
  // immediately with the value there, which may be the default of null.
  //
  // There is no order required for subscribe and send calls. You can send to
  // a mailbox with no subscribers, and a subscription can be registered for a
  // mailbox that has not been sent any data yet.
  var _Mailbox = function() {
    this.boxes = {};
  };

  // Subscribe to a mailbox of the name 'addr'. The callback 'cb' will
  // be called each time the mailbox is updated, including the very first time
  // a callback is registered, possibly with the default value of null.
  _Mailbox.prototype.subscribe = function(addr, cb) {
    var box = this.boxes[addr] || { callbacks: [], value: null };
    box.callbacks.push(cb);
    cb(box.value);
    this.boxes[addr] = box;
  };

  // Remove a callback from a subscription.
  _Mailbox.prototype.unsubscribe = function(addr, cb) {
    var box = this.boxes[addr] || { callbacks: [], value: null };
    // Use a countdown loop so multiple removals is safe.
    for (var i = box.callbacks.length-1; i >= 0; i--) {
      if (box.callbacks[i] == cb) {
        box.callbacks.splice(i, 1);
      }
    }
  };

  // Send data to a mailbox. All registered callbacks will be triggered
  // synchronously.
  _Mailbox.prototype.send = function(addr, value) {
    var box = this.boxes[addr] || { callbacks: [], value: null };
    box.value = value;
    this.boxes[addr] = box;
    box.callbacks.forEach(function(cb) {
      cb(value);
    });
  };

  // sk.Mailbox is an instance of sk._Mailbox, the only instance
  // that should be needed.
  sk.Mailbox = new _Mailbox();


  // Namespace for utilities for working with human i/o.
  sk.human = {};

  var DELTAS = [
    { units: "w", delta: 7*24*60*60 },
    { units: "d", delta:   24*60*60 },
    { units: "h", delta:      60*60 },
    { units: "m", delta:         60 },
    { units: "s", delta:          1 },
  ];

  /**
   * Returns a human-readable format of the given duration in seconds.
   *
   * For example, 'strDuration(123)' would return "2m 3s".
   *
   * Negative seconds is treated the same as positive seconds.
   */
  sk.human.strDuration = function(seconds) {
    if (seconds < 0) {
      seconds = -seconds;
    }
    if (seconds == 0) { return '  0s'; }
    var rv = "";
    for (var i=0; i<DELTAS.length; i++) {
      if (DELTAS[i].delta <= seconds) {
        var s = Math.floor(seconds/DELTAS[i].delta)+DELTAS[i].units;
        while (s.length < 4) {
          s = ' ' + s;
        }
        rv += s;
        seconds = seconds % DELTAS[i].delta;
      }
    }
    return rv;
  };

  /**
   * Returns the difference between the current time and 's' as a string in a
   * human friendly format.
   * If 's' is a number it is assumed to contain the time in milliseconds
   * otherwise it is assumed to contain a time string.
   *
   * For example, a difference of 123 seconds between 's' and the current time
   * would return "2m".
   */
  sk.human.diffDate = function(s) {
    var ms = (typeof(s) == "number") ? s : Date.parse(s);
    var diff = (ms - Date.now())/1000;
    if (diff < 0) {
      diff = -1.0 * diff;
    }
    for (var i=0; i<DELTAS.length-1; i++) {
      // If diff would round to '60s', return '1m' instead.
      var nextDeltaRounded =
          Math.round(diff/DELTAS[i+1].delta)*DELTAS[i+1].delta;
      if (nextDeltaRounded/DELTAS[i].delta >= 1) {
        return Math.round(diff/DELTAS[i].delta)+DELTAS[i].units;
      }
    }
    var i = DELTAS.length-1;
    return Math.round(diff/DELTAS[i].delta)+DELTAS[i].units;
  }

  // Namespace for utilities for working with arrays.
  sk.array = {};

  /**
   * Returns true if the two arrays are equal.
   *
   * Notes:
   *   Presumes the arrays are already in the same order.
   *   Compares equality using ===.
   */
  sk.array.equal = function(a, b) {
    if (a.length != b.length) {
      return false;
    }
    for (var i = 0, len = a.length; i < len; i++) {
      if (a[i] !== b[i]) {
        return false;
      }
    }
    return true;
  }

  /**
   * Formats the given string, replacing newlines with <br/> and auto-linkifying URLs.
   * References to bugs like "skia:123" and "chromium:123" are also converted into links.
   *
   * If linksInNewWindow is true, links are created with target="_blank".
   */
  sk.formatHTML = function(s, linksInNewWindow) {
    var sub = '<a href="$&">$&</a>';
    if (linksInNewWindow) {
      sub = '<a href="$&" target="_blank">$&</a>';
    }
    s = s.replace(/https?:\/\/[^ \t\n<]*/g, sub).replace(/(?:\r\n|\n|\r)/g, '<br/>');
    return sk.linkifyBugs(s);
  }

  var PROJECTS_TO_ISSUETRACKERS = {
    'chromium': 'http://crbug.com/',
    'skia': 'http://skbug.com/',
  }

  /**
   * Formats bug references like "skia:123" and "chromium:123" into links.
   */
  sk.linkifyBugs = function(s) {
    for (var project in PROJECTS_TO_ISSUETRACKERS) {
      var re = new RegExp(project + ":[0-9]+", "g");
      var found_bugs = s.match(re);
      if (found_bugs) {
        found_bugs.forEach(function(found_bug) {
          var bug_number = found_bug.split(":")[1];
          var bug_link = '<a href="' + PROJECTS_TO_ISSUETRACKERS[project] +
                         bug_number + '" target="_blank">' + found_bug +
                         '</a>';
          s = s.replace(found_bug, bug_link);
        });
      }
    }
    return s;
  }

  // Namespace for utilities for working with URL query strings.
  sk.query = {};


  // fromParamSet encodes an object of the form:
  //
  // {
  //   a:["2", "4"],
  //   b:["3"]
  // }
  //
  // to a query string like:
  //
  // "a=2&a=4&b=3"
  //
  // This function handles URI encoding of both keys and values.
  sk.query.fromParamSet = function(o) {
    if (!o) {
      return "";
    }
    var ret = [];
    var keys = Object.keys(o).sort();
    keys.forEach(function(key) {
      o[key].forEach(function(value) {
        ret.push(encodeURIComponent(key) + '=' + encodeURIComponent(value));
      });
    });
    return ret.join('&');
  }

  // toParamSet parses a query string into an object with
  // arrays of values for the values. I.e.
  //
  //   "a=2&b=3&a=4"
  //
  // decodes to
  //
  //   {
  //     a:["2", "4"],
  //     b:["3"],
  //   }
  //
  // This function handles URI decoding of both keys and values.
  sk.query.toParamSet = function(s) {
    s = s || '';
    var ret = {};
    var vars = s.split("&");
    for (var i=0; i<vars.length; i++) {
      var pair = vars[i].split("=", 2);
      if (pair.length == 2) {
        var key = decodeURIComponent(pair[0]);
        var value = decodeURIComponent(pair[1]);
        if (ret.hasOwnProperty(key)) {
          ret[key].push(value);
        } else {
          ret[key] = [value];
        }
      }
    }
    return ret;
  }


  // fromObject takes an object and encodes it into a query string.
  //
  // The reverse of this function is toObject.
  sk.query.fromObject = function(o) {
    var ret = [];
    Object.keys(o).forEach(function(key) {
      ret.push(encodeURIComponent(key) + '=' + encodeURIComponent(o[key]));
    });
    return ret.join('&');
  }


  // toObject decodes a query string into an object
  // using the 'target' as a source for hinting on the types
  // of the values.
  //
  //   "a=2&b=true"
  //
  // decodes to:
  //
  //   {
  //     a: 2,
  //     b: true,
  //   }
  //
  // When given a target of:
  //
  //   {
  //     a: 1.0,
  //     b: false,
  //   }
  //
  // Note that a target of {} would decode
  // the same query string into:
  //
  //   {
  //     a: "2",
  //     b: "true",
  //   }
  //
  // Only Number, String, and Boolean hints are supported.
  sk.query.toObject = function(s, target) {
    var target = target || {};
    var ret = {};
    var vars = s.split("&");
    for (var i=0; i<vars.length; i++) {
      var pair = vars[i].split("=", 2);
      if (pair.length == 2) {
        var key = decodeURIComponent(pair[0]);
        var value = decodeURIComponent(pair[1]);
        if (target.hasOwnProperty(key)) {
          switch (typeof(target[key])) {
            case 'boolean':
              ret[key] = value=="true";
              break;
            case 'number':
              ret[key] = Number(value);
              break;
            case 'string':
              ret[key] = value;
              break;
            default:
              ret[key] = value;
          }
        } else {
          ret[key] = value;
        }
      }
    }
    return ret;
  }

  // Namespace for utilities for working with Objects.
  sk.object = {};

  // Returns an object with only values that are in o that are different
  // from d.
  sk.object.getDelta = function (o, d) {
    var ret = {};
    Object.keys(o).forEach(function(key) {
      if (o[key] != d[key]) {
        ret[key] = o[key];
      }
    });
    return ret;
  }

  // Returns a copy of object o with values from delta if they exist.
  sk.object.applyDelta = function (delta, o) {
    var ret = {};
    Object.keys(o).forEach(function(key) {
      if (delta.hasOwnProperty(key)) {
        ret[key] = delta[key];
      } else {
        ret[key] = o[key];
      }
    });
    return ret;
  }

  // Track the state of a page and reflect it to and from the URL.
  //
  // page - An object with a property 'state' where the state to be reflected
  //        into the URL is stored. We need the level of indirection because
  //        JS doesn't have pointers.
  // cb   - A callback of the form function() that is called when state has been
  //        changed by a change in the URL.
  sk.stateReflector = function(page, cb) {
    // The default state of the page. Used to calculate diffs to state.
    var defaultState = JSON.parse(JSON.stringify(page.state));

    // The last state of the page. Used to determine if the page state has changed recently.
    var lastState = JSON.parse(JSON.stringify(page.state));

    // Watch for state changes and reflect them in the URL by simply
    // polling the object and looking for differences from defaultState.
    setInterval(function() {
      if (Object.keys(sk.object.getDelta(lastState, page.state)).length > 0) {
        lastState = JSON.parse(JSON.stringify(page.state));
        var q = sk.query.fromObject(sk.object.getDelta(page.state, defaultState));
        history.pushState(null, "", window.location.origin + window.location.pathname + "?" +  q);
      }
    }, 100);

    // stateFromURL should be called when the URL has changed, it updates
    // the page.state and triggers the callback.
    var stateFromURL = function() {
      var delta = sk.query.toObject(window.location.search.slice(1), defaultState);
      page.state = sk.object.applyDelta(delta, defaultState);
      lastState = JSON.parse(JSON.stringify(page.state));
      cb();
    }

    // When we are loaded we should update the state from the URL.
    sk.WebComponentsReady.then(stateFromURL);

    // Every popstate event should also update the state.
    window.addEventListener('popstate', stateFromURL);
  }

  // Find a "round" number in the given range. Attempts to find numbers which
  // consist of a multiple of one of the following, order of preference:
  // [5, 2, 1], followed by zeroes.
  //
  // TODO(borenet): It would be nice to support other multiples, for example,
  // when dealing with time data, it'd be nice to round to seconds, minutes,
  // hours, days, etc.
  sk.getRoundNumber = function(min, max, base) {
    if (min > max) {
      throw ("sk.getRoundNumber: min > max! (" + min + " > " + max + ")");
    }
    var multipleOf = [5, 2, 1];

    var val = (max + min) / 2;
    // Determine the number of digits left of the decimal.
    if (!base) {
      base = 10;
    }
    var digits = Math.floor(Math.log(Math.abs(val)) / Math.log(base)) + 1;

    // Start with just the most significant digit and attempt to round it to
    // multiples of the above numbers, gradually including more digits until
    // a "round" value is found within the given range.
    for (var shift = 0; ; shift++) {
      // Round by shifting digits and dividing by a multiplier, then performing
      // the round function, then multiplying and shifting back.
      var shiftDiv = Math.pow(base, (digits - shift));
      for (var i = 0; i < multipleOf.length; i++) {
        var f = shiftDiv * multipleOf[i];
        // Actually perform the rounding. The 10s are included to intentionally
        // reduce precision to round off floating point error.
        var newVal = ((Math.round(val / f) * 10) * f) / 10;
        if (newVal >= min && newVal <= max) {
          return newVal;
        }
      }
    }

    console.error("sk.getRoundNumber Couldn't find appropriate rounding " +
                  "value. Returning midpoint.");
    return val;
  }

  // Sort the given array of strings, ignoring case.
  sk.sortStrings = function(s) {
    return s.sort(function(a, b) {
      return a.localeCompare(b, "en", {"sensitivity": "base"});
    });
  }

  // Capitalize each word in the string.
  sk.toCapWords = function(s) {
    return s.replace(/\b\w/g, function(firstLetter) {
      return firstLetter.toUpperCase();
    });
  }

  // Polyfill for String.startsWith from 
  // https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/String/startsWith#Polyfill
  // returns true iff the string starts with the given prefix
  if (!String.prototype.startsWith) {
    String.prototype.startsWith = function(searchString, position) {
      position = position || 0;
      return this.indexOf(searchString, position) === position;
    };
  }

  return sk;
}();
