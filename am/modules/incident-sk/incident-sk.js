/**
 * @module incident-sk
 * @description <h2><code>incident-sk</code></h2>
 *
 * <p>
 *   Displays a single Incident.
 * </p>
 *
 * @attr minimized {boolean} If not set then the incident is displayed in expanded
 *    mode, otherwise it is displayed in compact mode.
 *
 * @attr params {boolean} If set then the incident params are displayed, only
 *    applicable if minimzed is true.
 *
 * @evt add-note Sent when the user adds a note to an incident.
 *    The detail includes the text of the note and the key of the incident.
 *
 *   <pre>
 *     detail {
 *       key: "12312123123",
 *       text: "blah blah blah",
 *     }
 *   </pre>
 *
 * @evt del-note Sent when the user deletes a note on an incident.
 *    The detail includes the index of the note and the key of the incident.
 *
 *   <pre>
 *     detail {
 *       key: "12312123123",
 *       index: 0,
 *     }
 *   </pre>
 *
 * @evt take Sent when the user wants the incident assigned to themselves.
 *    The detail includes the key of the incident.
 *
 *   <pre>
 *     detail {
 *       key: "12312123123",
 *     }
 *   </pre>
 *
 * @evt assign Sent when the user want to assign the incident to someone else.
 *    The detail includes the key of the incident.
 *
 *   <pre>
 *     detail {
 *       key: "12312123123",
 *     }
 *   </pre>
 *
 */
import 'elements-sk/icon/delete-icon-sk'
import '../silence-sk'

import * as paramset from '../paramset'
import { $$ } from 'common-sk/modules/dom'
import { abbr, linkify, notes } from '../am'
import { diffDate, strDuration } from 'common-sk/modules/human'
import { errorMessage } from 'elements-sk/errorMessage'
import { html, render } from 'lit-html'
import { jsonOrThrow } from 'common-sk/modules/jsonOrThrow'

function classOfH2(ele) {
  if (!ele._state.active) {
    return 'inactive';
  }
  if (ele._state.params.assigned_to) {
    return 'assigned';
  }
}

function table(o) {
  let keys = Object.keys(o);
  keys.sort();
  return keys.filter(k => !k.startsWith('__')).map((k) => html`<tr><th>${k}</th><td>${linkify(o[k])}</td></tr>`);
}

function actionButtons(ele) {
  if (ele._state.active) {
    return html`<section class=assign>
      <button @click=${ele._take}>Take</button>
      <button @click=${ele._assign}>Assign</button>
    </section>`;
  } else {
    return html``;
  }
}

function matchingSilences(ele) {
  if (ele.hasAttribute('minimized')) {
    return ``;
  }
  let ret = ele._silences.filter(silence => paramset.match(silence.param_set, ele._state.params)).map( silence =>
    html`<silence-sk state=${silence} collapsable collapsed></silence-sk>`
  );
  if (!ret.length) {
    ret.push(html`<div class=nosilences>None</div>`);
  }
  return ret;
}

function lastSeen(ele) {
  if (ele._state.active) {
    return '';
  } else {
    return html`<tr><th>Last Seen</th><td title=${new Date(ele._state.last_seen*1000).toLocaleString()}>${diffDate(ele._state.last_seen*1000)}</td></tr>`;
  }
}

function duration(ele) {
  if (ele._state.active) {
    return '';
  } else {
    return html`<tr><th>Duration</th><td>${strDuration(ele._state.last_seen - ele._state.start)}</td></tr>`;
  }
}

function history(ele) {
  if (ele.hasAttribute('minimized')) {
    return ``;
  }
  return fetch(`/_/recent_incidents?id=${ele._state.id}&key=${ele._state.key}`, {
    headers: {
      'content-type': 'application/json',
    },
    credentials: 'include',
    method: 'GET',
  }).then(jsonOrThrow).then(json => {
    json = json || [];
    return json.map(i => html`<incident-sk state=${i} minimized></incident-sk>`);
  }).catch(errorMessage);
}

const template = (ele) => html`
  <h2 class=${classOfH2(ele)}>${ele._state.params.alertname} ${abbr(ele._state)}</h2>
  <section class=detail>
    ${actionButtons(ele)}
    <table class=timing>
      <tr><th>Started</th><td title=${new Date(ele._state.start*1000).toLocaleString()}>${diffDate(ele._state.start*1000)}</td></tr>
      ${lastSeen(ele)}
      ${duration(ele)}
    </table>
    <table class=params>
      ${table(ele._state.params)}
    </table>
    ${notes(ele)}
    <section class=addNote>
      <textarea rows=2 cols=80></textarea>
      <button @click=${ele._addNote}>Submit</button>
    </section>
    <section class=matchingSilences>
      <h3>Matching Silences</h3>
      ${matchingSilences(ele)}
    </section>
    <section class=history>
      <h3>History</h3>
      ${history(ele)}
    </section>
  </section>
`;

window.customElements.define('incident-sk', class extends HTMLElement {
  constructor() {
    super();
    this._silences = [];
  }

  /** @prop state {Object} An Incident. */
  get state() { return this._state }
  set state(val) {
    this._state = val;
    this._render();
  }

  /** @prop silences {string} The list of active silences. */
  get silences() { return this._silences }
  set silences(val) {
    this._render();
    this._silences = val;
  }

  _take(e) {
    let detail = {
      key: this._state.key,
    };
    this.dispatchEvent(new CustomEvent('take', { detail: detail, bubbles: true }));
  }

  _assign(e) {
    let detail = {
      key: this._state.key,
    };
    this.dispatchEvent(new CustomEvent('assign', { detail: detail, bubbles: true }));
  }

  _deleteNote(e, index) {
    let detail = {
      key: this._state.key,
      index: index,
    };
    this.dispatchEvent(new CustomEvent('del-note', { detail: detail, bubbles: true }));
  }

  _addNote(e) {
    let textarea = $$('textarea', this);
    let detail = {
      key: this._state.key,
      text: textarea.value,
    };
    this.dispatchEvent(new CustomEvent('add-note', { detail: detail, bubbles: true }));
    textarea.value = '';
  }

  _render() {
    if (!this._state) {
      return
    }
    render(template(this), this, {eventContext: this});
  }

});
