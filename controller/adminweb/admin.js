'use strict';
const $ = (selector) => document.querySelector(selector);
const state = { csrf: '', role: '', selectedCase: null, records: [], curationKind: '', editingRecord: null, curationRequest: 0, numberOffset: 0, numberQuery: '', numberRequests: 0, numberDetailRequests: 0 };
function node(tag, text, className) { const el = document.createElement(tag); if (text !== undefined) el.textContent = String(text ?? '—'); if (className) el.className = className; return el; }
function status(message, error = false) { const el = $('#status'); el.textContent = message; el.classList.toggle('error', error); el.hidden = false; }
function clearStatus() { $('#status').hidden = true; }
async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (options.body && !(options.body instanceof FormData)) headers.set('Content-Type', 'application/json');
  if (options.method && !['GET', 'HEAD'].includes(options.method)) headers.set('X-CSRF-Token', state.csrf);
  const response = await fetch('/api/v1/admin' + path, { ...options, headers, credentials: 'same-origin', cache: 'no-store' });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok || payload.success === false) {
    if (response.status === 403 && path !== '/login') await restoreSession(false);
    throw new Error(payload.message || `Permintaan gagal (${response.status}).`);
  }
  return payload.data;
}
async function action(button, task) { clearStatus(); const old = button?.disabled; if (button) button.disabled = true; try { await task(); } catch (error) { status(error.message || 'Permintaan gagal.', true); } finally { if (button) button.disabled = old; } }
function items(data) { return Array.isArray(data) ? data : (data?.items || data?.records || []); }
function date(value) { if (!value) return '—'; const d = new Date(value); return Number.isNaN(d.valueOf()) ? String(value) : d.toLocaleString('id-ID'); }
function tab(name) { document.querySelectorAll('[id^="tab-"]').forEach(el => { el.hidden = el.id !== `tab-${name}`; }); document.querySelectorAll('[data-tab]').forEach(el => { if (el.dataset.tab === name) el.setAttribute('aria-current', 'page'); else el.removeAttribute('aria-current'); }); }
function setSession(session) {
  state.csrf = session?.csrf_token || ''; state.role = session?.role || '';
  $('#login-panel').hidden = !!session; $('#workspace').hidden = !session; $('#logout').hidden = !session;
  $('#identity').textContent = session ? `@${session.username || ''} · ${session.role}` : '';
  document.querySelectorAll('[data-admin]').forEach(el => { el.hidden = state.role !== 'ADMIN'; });
  if (!session) { state.selectedCase = null; state.records = []; state.curationKind = ''; state.curationRequest++; closeEditor(); $('#case-detail').hidden = true; $('#case-rows').replaceChildren(); $('#audit-rows').replaceChildren(); $('#curation-list').replaceChildren(); $('#number-result').replaceChildren(); $('#number-result').hidden = true; $('#number-rows').replaceChildren(); state.numberRequests++; state.numberDetailRequests++; tab('cases'); }
}
async function restoreSession(load = true) {
  try { const res = await fetch('/api/v1/admin/auth/session', { credentials: 'same-origin', cache: 'no-store' }); if (!res.ok) { setSession(null); return; } const body = await res.json(); setSession(body.data); } catch { setSession(null); return; }
  if (load) await action(null, loadCases);
}
$('#login-form').addEventListener('submit', event => { event.preventDefault(); const form = event.currentTarget; action(form.querySelector('button'), async () => { const data = await api('/login', { method: 'POST', body: JSON.stringify(Object.fromEntries(new FormData(form))) }); form.reset(); setSession(data); if (!state.csrf) await restoreSession(false); await loadCases(); }); });
$('#logout').addEventListener('click', event => action(event.currentTarget, async () => { await api('/logout', { method: 'POST', body: '{}' }); setSession(null); status('Sesi staf ditutup.'); }));
$('#refresh').addEventListener('click', event => {
  const current = document.querySelector('[data-tab][aria-current="page"]').dataset.tab;
  const load = { cases: loadCases, numbers: loadNumbers, curation: loadCuration, audit: loadAudit };
  action(event.currentTarget, () => load[current]());
});
document.querySelectorAll('[data-tab]').forEach(button => button.addEventListener('click', () => action(button, async () => { tab(button.dataset.tab); if (button.dataset.tab === 'numbers') await loadNumbers(); if (button.dataset.tab === 'audit') await loadAudit(); if (button.dataset.tab === 'curation') await loadCuration(); })));
$('#filter-form').addEventListener('submit', event => { event.preventDefault(); action(event.currentTarget.querySelector('button'), loadCases); });
const caseTypes = { DISPUTE: 'Sanggahan', REPORT_REVIEW: 'Laporan' };
const caseStatuses = { OPEN: 'Baru', UNDER_REVIEW: 'Ditinjau', NEEDS_INFO: 'Butuh informasi', PROCESSING: 'Diproses', ACCEPTED: 'Diterima', REJECTED: 'Ditolak' };
const activeStatuses = new Set(['OPEN', 'UNDER_REVIEW', 'NEEDS_INFO', 'PROCESSING']);
function ageLabel(submitted, type) {
  const timestamp = Date.parse(submitted);
  if (!Number.isFinite(timestamp) || timestamp <= 0) return null;
  const hours = Math.max(0, (Date.now() - timestamp) / 3600000);
  const days = Math.floor(hours / 24);
  const age = days ? `${days} hari` : hours >= 1 ? `${Math.floor(hours)} jam` : 'Kurang dari 1 jam';
  const targetHours = { DISPUTE: 168, REPORT_REVIEW: 72 };
  const overdue = hours > targetHours[type];
  return node('small', overdue ? `${age} · Lewat target` : age, overdue ? 'case-age overdue' : 'case-age');
}
async function loadCases() {
  const query = new URLSearchParams(new FormData($('#filter-form')));
  const data = await api('/cases?' + query); const summary = data?.summary || {};
  const metrics = [['queue_count', 'Kasus aktif'], ['overdue_disputes', 'Sanggahan lewat 7 hari'], ['overdue_reports', 'Laporan lewat 3 hari'], ['reports_24h', 'Laporan 24 jam']];
  $('#metrics').replaceChildren(...metrics.map(([key, label]) => { const card = node('div', undefined, 'metric'); card.append(node('strong', summary[key] ?? '—'), node('span', label)); return card; }));
  const cases = items(data);
  $('#case-count').textContent = `${cases.length} kasus ditampilkan${cases.length === 100 ? ' (batas 100 per filter)' : ''}`;
  const rows = cases.map(item => {
    const row = node('tr'); const key = item.dispute_id || item.id;
    const target = node('td'); target.append(node('div', item.phone_e164), node('small', key));
    const control = node('td'); const button = node('button', 'Tinjau', 'quiet'); button.addEventListener('click', () => action(button, () => openCase(key))); control.append(button);
    const submitted = item.submitted_at || item.created_at;
    const timing = node('td', date(submitted));
    if (activeStatuses.has(item.status)) {
      const age = ageLabel(submitted, item.type);
      if (age) timing.append(age);
    }
    const cell = node('td');
    cell.append(node('span', caseStatuses[item.status] || item.status, 'case-status'));
    row.append(target, node('td', caseTypes[item.type] || item.type), cell, timing, control); return row;
  });
  if (!rows.length) { const row = node('tr'); const cell = node('td', 'Tidak ada kasus untuk filter ini.'); cell.colSpan = 5; row.append(cell); rows.push(row); }
  $('#case-rows').replaceChildren(...rows);
}
async function openCase(id) {
  const data = await api('/cases/' + encodeURIComponent(id)); const item = data?.case || data;
  state.selectedCase = item; $('#case-title').textContent = `${caseTypes[item.type] || item.type} · ${item.phone_e164}`;
  const detail = $('#case-evidence'); detail.replaceChildren();
  for (const [label, value] of [['ID kasus', item.dispute_id || item.id], ['Status', caseStatuses[item.status] || item.status], ['Versi', item.version], ['Diajukan', date(item.submitted_at)], ['Alasan pemohon', item.reason], ['Nama layanan/pemilik', item.owner_name], ['Hasil sebelumnya', item.resolution_notes]]) detail.append(node('dt', label), node('dd', value));
  const evidence = $('#related-reports'); evidence.replaceChildren(node('h3', 'Laporan terkait'));
  const reports = data?.reports || [];
  if (!reports.length) evidence.append(node('p', 'Belum ada laporan terkait.'));
  reports.forEach(report => { const entry = node('div'); entry.append(node('strong', report.custom_tag || report.tag_name || report.category_code), node('p', report.comment || 'Tanpa komentar')); evidence.append(entry); });
  $('#decision-form').reset();
  const isOpen = ['OPEN', 'UNDER_REVIEW', 'NEEDS_INFO'].includes(item.status);
  $('#decision-form').hidden = !isOpen;
  const confirmOpt = $('#decision-form option[value="CONFIRM_DANGER"]');
  if (confirmOpt) confirmOpt.hidden = item.type !== 'REPORT_REVIEW';
  $('#case-detail').hidden = false; $('#case-detail').scrollIntoView({ block: 'start' });
}
$('#decision-form').addEventListener('submit', event => { event.preventDefault(); const form = event.currentTarget; action(form.querySelector('button'), async () => { const current = state.selectedCase; if (!current) throw new Error('Pilih kasus lebih dulu.'); const input = Object.fromEntries(new FormData(form)); input.version = current.version; await api('/cases/' + encodeURIComponent(current.dispute_id || current.id), { method: 'PATCH', body: JSON.stringify(input) }); $('#case-detail').hidden = true; await loadCases(); status('Keputusan disimpan dan perubahan masuk sinkronisasi.'); }); });

const reputationLabels = { CLEAN: 'Bersih', SUSPICIOUS: 'Mencurigakan', SPAM: 'Spam', DANGER_SCAM: 'Bahaya penipuan' };
function maskedPhone(value) {
  return value.length > 8 ? value.slice(0, 5) + '••••' + value.slice(-4) : value;
}
async function loadNumbers(offset = state.numberOffset, search = state.numberQuery) {
  const request = ++state.numberRequests;
  const params = new URLSearchParams(search);
  params.set('offset', offset);
  const data = await api('/numbers?' + params);
  if (request !== state.numberRequests || !state.role) return;
  state.numberOffset = offset;
  state.numberQuery = search;
  state.numberDetailRequests++;
  const detail = $('#number-result');
  detail.hidden = true;
  detail.replaceChildren();
  const entries = items(data);
  $('#number-count').textContent = entries.length ? `Menampilkan entri ${offset + 1}–${offset + entries.length}` : 'Tidak ada hasil untuk pencarian ini.';
  $('#number-pagination').hidden = !offset && !data.more;
  $('#number-prev').disabled = offset === 0;
  $('#number-next').disabled = !data.more;
  $('#number-rows').replaceChildren(...entries.map(entry => {
    const button = node('button', 'Lihat detail', 'quiet');
    button.type = 'button';
    button.setAttribute('aria-label', 'Lihat detail ' + maskedPhone(entry.phone_e164));
    button.addEventListener('click', () => action(button, () => openNumber(entry.phone_e164)));
    const actions = node('td');
    actions.append(button);
    const row = node('tr');
    const label = entry.top_community_label || entry.tags?.[0]?.tag || entry.primary_category_label;
    row.append(node('td', maskedPhone(entry.phone_e164)), node('td', label || '—'),
      node('td', reputationLabels[entry.reputation_level] || '—'), node('td', entry.total_reports ?? 0), actions);
    return row;
  }));
}
async function openNumber(number) {
  const request = ++state.numberDetailRequests;
  const data = await api('/numbers/' + encodeURIComponent(number));
  if (request !== state.numberDetailRequests || !state.role) return;
  const detail = $('#number-result');
  const phone = data.phone || {};
  const title = node('h3', number);
  title.tabIndex = -1;
  const summary = node('dl');
  for (const [label, value] of [
    ['Reputasi', reputationLabels[phone.reputation_level] || '—'],
    ['Kategori', phone.primary_category_label || '—'],
    ['Jumlah laporan', phone.total_reports ?? 0],
    ['Terakhir dilaporkan', date(phone.last_reported_at)]
  ]) summary.append(node('dt', label), node('dd', value));
  detail.replaceChildren(title, summary, node('h4', 'Tag komunitas'));
  if (phone.tags?.length) {
    const tags = node('ul');
    tags.append(...phone.tags.map(tag => node('li', `${tag.tag} · ${tag.count} suara`)));
    detail.append(tags);
  } else {
    detail.append(node('p', 'Belum ada tag komunitas.', 'muted'));
  }
  detail.append(node('h4', 'Laporan dan kasus'));
  const evidence = node('ul');
  for (const report of data.reports || []) {
    evidence.append(node('li', [report.custom_tag || report.tag_name || report.category_code,
      report.status, date(report.created_at)].filter(Boolean).join(' · ')));
  }
  for (const item of data.cases || []) {
    const line = node('li');
    const link = node('button', `Tinjau ${caseTypes[item.type] || 'kasus'} · ${caseStatuses[item.status] || item.status}`, 'quiet');
    link.addEventListener('click', () => action(link, async () => {
      tab('cases');
      await openCase(item.dispute_id || item.id);
    }));
    line.append(link);
    evidence.append(line);
  }
  detail.append(evidence.childElementCount ? evidence : node('p', 'Belum ada laporan atau kasus.', 'muted'));
  detail.hidden = false;
  title.focus();
}
$('#number-form').addEventListener('submit', event => {
  event.preventDefault();
  const form = event.currentTarget;
  action(form.querySelector('button'), () => loadNumbers(0, new URLSearchParams(new FormData(form)).toString()));
});
$('#number-prev').addEventListener('click', () => action(null, () => loadNumbers(Math.max(0, state.numberOffset - 50))));
$('#number-next').addEventListener('click', () => action(null, () => loadNumbers(state.numberOffset + 50)));
const curationFields = {
  'verified-numbers': [
    ['phone_e164', 'Nomor telepon', 'tel', true],
    ['display_label', 'Nama yang ditampilkan', 'text', true],
    ['kind', 'Kategori', ['BUSINESS', 'OFFICIAL', 'PUBLIC_SERVICE']],
    ['status', 'Status', ['ACTIVE', 'INACTIVE', 'REVOKED']],
    ['expires_at', 'Berlaku sampai', 'datetime-local', true]
  ],
  emergency: [
    ['code', 'Kode entri', 'text', true],
    ['name', 'Nama layanan', 'text', true],
    ['scope', 'Cakupan', ['LOCAL', 'NATIONAL']],
    ['agency_type', 'Jenis instansi', ['DAMKAR', 'TERPADU', 'POLISI', 'AMBULANS_RS', 'SAR', 'PLN', 'JASA_MARGA']],
    ['province', 'Provinsi', 'text'],
    ['city_regency', 'Kabupaten / kota', 'text'],
    ['contacts.short_code', 'Nomor pendek', 'tel'],
    ['contacts.phone_numbers', 'Nomor telepon (satu per baris)', 'list'],
    ['contacts.whatsapp_number', 'Nomor WhatsApp', 'tel'],
    ['contacts.whatsapp_template', 'Pesan WhatsApp', 'textarea'],
    ['capabilities', 'Layanan (satu per baris)', 'list'],
    ['address', 'Alamat', 'text'],
    ['location', 'Koordinat posko (bujur, lintang)', 'coordinates'],
    ['is_24_hours', 'Tersedia 24 jam', 'checkbox']
  ],
  'official-channels': [
    ['channel_slug', 'Kode kanal', 'text', true],
    ['title', 'Nama kanal', 'text', true],
    ['authority_name', 'Instansi', 'text', true],
    ['description', 'Deskripsi', 'textarea'],
    ['action_links.website_url', 'Situs resmi (HTTPS)', 'url', true],
    ['action_links.phone_dial', 'Nomor telepon', 'tel'],
    ['action_links.whatsapp_url', 'Tautan WhatsApp (HTTPS)', 'url'],
    ['action_links.email', 'Email layanan', 'email'],
    ['related_categories', 'Kode kategori (satu per baris)', 'list'],
    ['required_evidence_checklist', 'Bukti yang dibutuhkan (satu per baris)', 'list'],
    ['step_by_step_guide', 'Langkah pelaporan (satu per baris)', 'list'],
    ['display_order', 'Urutan tampil', 'number'],
    ['is_active', 'Kanal aktif', 'checkbox']
  ]
};
const sourceFields = [
  ['source_url', 'Tautan sumber verifikasi (HTTPS)', 'url', true],
  ['verified_at', 'Terakhir diverifikasi', 'datetime-local', true]
];
const fieldLabels = {
  BUSINESS: 'Bisnis', OFFICIAL: 'Instansi resmi', PUBLIC_SERVICE: 'Layanan publik',
  ACTIVE: 'Aktif', INACTIVE: 'Nonaktif', REVOKED: 'Dicabut',
  LOCAL: 'Daerah', NATIONAL: 'Nasional', AMBULANS_RS: 'Ambulans / rumah sakit', JASA_MARGA: 'Jasa Marga'
};
function fieldValue(record, path) {
  return path.split('.').reduce((value, key) => value?.[key], record);
}
function localDateTime(value) {
  if (!value) return '';
  const time = new Date(value);
  if (Number.isNaN(time.valueOf()) || time.getFullYear() < 1900) return '';
  return new Date(time.getTime() - time.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}
function curationField(field, values, immutable) {
  const [path, title, type, required] = field;
  const value = fieldValue(values, path);
  const label = node('label', title);
  let input;
  if (Array.isArray(type)) {
    input = node('select');
    const options = value && !type.includes(value) ? [...type, value] : type;
    for (const option of options) {
      const item = node('option', fieldLabels[option] || option);
      item.value = option;
      input.append(item);
    }
    input.value = value ?? type[0];
  } else if (type === 'list' || type === 'textarea') {
    input = node('textarea');
    input.rows = 3;
    input.value = type === 'list' ? (value || []).join('\n') : (value ?? '');
  } else {
    input = node('input');
    input.type = type === 'coordinates' ? 'text' : type;
    if (type === 'checkbox') {
      label.className = 'check';
      input.checked = value ?? true;
    } else if (type === 'coordinates') {
      input.value = value?.coordinates?.join(', ') || '';
      input.placeholder = '106.8272, -6.1754';
      label.append(node('small', 'Isi agar posko dapat diurutkan berdasarkan jarak di HP.'));
    } else if (type === 'datetime-local') {
      input.value = localDateTime(value);
      input.defaultValue = input.value;
    } else {
      input.value = value ?? (type === 'number' ? '0' : '');
    }
  }
  input.name = path;
  input.readOnly = immutable;
  input.required = Boolean(required) && !immutable;
  label.append(input);
  return label;
}
function recordId(record, kind) {
  return fieldValue(record, curationFields[kind][0][0]);
}
function recordTitle(record, kind) {
  return record.display_label || record.name || record.title || recordId(record, kind);
}
function closeEditor() {
  state.editingRecord = null;
  if ($('#emergency-preview').open) $('#emergency-preview').close();
  $('#curation-form').hidden = true;
  $('#curation-fields').replaceChildren();
  $('#curation-form').elements.reason.value = '';
}
function openEditor(record = null) {
  const kind = state.curationKind;
  if (!kind) return;
  const now = new Date();
  const values = record || { verified_at: now.toISOString(), expires_at: new Date(now.getTime() + 180 * 86400000).toISOString() };
  const form = $('#curation-form');
  state.editingRecord = record;
  $('#curation-title').textContent = record ? recordTitle(record, kind) : 'Entri baru';
  $('#curation-fields').replaceChildren(...[...curationFields[kind], ...sourceFields].map((field, index) =>
    curationField(field, values, Boolean(record) && index === 0)
  ));
  form.elements.reason.value = '';
  $('#save-record').textContent = kind === 'emergency' ? 'Pratinjau publikasi' : 'Simpan entri';
  if (kind === 'emergency') {
    const scope = form.elements.scope;
    const updateRequired = () => {
      const local = scope.value === 'LOCAL';
      form.elements.province.required = local;
      form.elements.city_regency.required = local;
    };
    scope.addEventListener('change', updateRequired);
    updateRequired();
  }
  const remove = $('#delete-record');
  remove.hidden = !record;
  remove.textContent = kind === 'emergency' ? 'Hapus entri' : 'Cabut entri';
  form.hidden = false;
  form.elements[curationFields[kind][record ? 1 : 0][0]].focus();
}
function renderCurationList() {
  const kind = state.curationKind;
  const query = $('#curation-search').value.trim().toLowerCase();
  const list = $('#curation-list');
  const matches = state.records.filter(record =>
    [recordTitle(record, kind), recordId(record, kind), record.province, record.city_regency, record.address,
      record.agency_type, record.authority_name, record.contacts?.short_code, record.contacts?.whatsapp_number,
      ...(record.contacts?.phone_numbers || []), record.action_links?.phone_dial]
      .some(value => typeof value === 'string' && value.toLowerCase().includes(query))
  );
  $('#curation-count').textContent = `${matches.length} dari ${state.records.length} entri`;
  list.replaceChildren(...matches.map(record => {
    const button = node('button');
    button.type = 'button';
    button.append(node('strong', recordTitle(record, kind)), node('small', recordId(record, kind)));
    button.addEventListener('click', () => openEditor(record));
    return button;
  }));
  if (!matches.length) {
    list.append(node('p', query ? 'Tidak ada entri yang cocok.' : 'Dataset belum memiliki entri.', 'muted'));
  }
}
async function loadCuration() {
  const kind = $('#curation-kind').value;
  const request = ++state.curationRequest;
  state.curationKind = '';
  state.records = [];
  closeEditor();
  $('#new-record').disabled = true;
  $('#curation-search').value = '';
  $('#curation-list').replaceChildren();
  const records = items(await api('/' + kind));
  if (request !== state.curationRequest || state.role !== 'ADMIN') return;
  state.curationKind = kind;
  state.records = records;
  $('#new-record').disabled = false;
  $('#new-record').textContent = kind === 'emergency' ? 'Tambah posko atau nomor' : 'Entri baru';
  renderCurationList();
}
function curationValue(input, type, original) {
  if (type === 'list') return input.value.split('\n').map(line => line.trim()).filter(Boolean);
  if (type === 'checkbox') return input.checked;
  if (type === 'number') return input.valueAsNumber || 0;
  if (type === 'coordinates') {
    if (!input.value.trim()) return null;
    const parts = input.value.split(',').map(part => part.trim());
    const coordinates = parts.map(Number);
    if (parts.length !== 2 || parts.some(part => !part) || !coordinates.every(Number.isFinite)
      || Math.abs(coordinates[0]) > 180 || Math.abs(coordinates[1]) > 90) {
      throw new Error('Isi koordinat dalam format bujur, lintang.');
    }
    return { type: 'Point', coordinates };
  }
  if (type === 'datetime-local') {
    return original && input.value === input.defaultValue ? original : new Date(input.value).toISOString();
  }
  return input.value.trim();
}
$('#curation-kind').addEventListener('change', () => action(null, loadCuration));
$('#curation-search').addEventListener('input', () => { if (state.curationKind) renderCurationList(); });
$('#new-record').addEventListener('click', () => openEditor());
$('#cancel-record').addEventListener('click', () => {
  closeEditor();
  $('#new-record').focus();
});
const emergencyPreview = $('#emergency-preview');
let emergencyDraft;
async function saveCuration(kind, id, body) {
  const path = '/' + kind + (id ? '/' + encodeURIComponent(id) : '');
  await api(path, { method: id ? 'PUT' : 'POST', body: JSON.stringify(body) });
}
function showEmergencyPreview(id, body) {
  const phones = [body.contacts.short_code, ...body.contacts.phone_numbers].filter(Boolean);
  const whatsapp = body.contacts.whatsapp_number;
  if (!phones.length && !whatsapp) throw new Error('Isi setidaknya satu nomor telepon, nomor pendek, atau WhatsApp.');
  const validNumber = /^(?:\+[1-9][0-9]{6,14}|[1-9][0-9]{2,5}|150[0-9]{4})$/;
  if ([...phones, whatsapp].filter(Boolean).some(number => !validNumber.test(number))) {
    throw new Error('Gunakan format +62 untuk nomor Indonesia lengkap, atau masukkan nomor pendek resmi.');
  }
  const source = new URL(body.source_url);
  if (source.protocol !== 'https:' || source.username || source.password) {
    throw new Error('Gunakan tautan HTTPS dari sumber resmi.');
  }
  const fields = $('#emergency-preview-fields');
  fields.replaceChildren();
  for (const [label, value] of [
    ['Nama layanan', body.name],
    ['Cakupan', body.scope === 'NATIONAL' ? 'Nasional' : [body.city_regency, body.province].join(', ')],
    ['Telepon', [...new Set(phones)].join(', ')],
    ['WhatsApp', whatsapp],
    ['Alamat', body.address],
    ['Jam layanan', body.is_24_hours ? 'Siaga 24 jam' : 'Sesuai jam kerja'],
    ['Urutan jarak', body.location ? body.location.coordinates.join(', ') : 'Koordinat belum diisi'],
    ['Sumber resmi', body.source_url],
    ['Diverifikasi', date(body.verified_at)]
  ]) {
    if (value) fields.append(node('dt', label), node('dd', value));
  }
  emergencyDraft = { id, body };
  $('#emergency-preview-error').hidden = true;
  emergencyPreview.showModal();
  $('#edit-emergency').focus();
}
emergencyPreview.addEventListener('close', () => { emergencyDraft = undefined; });
emergencyPreview.addEventListener('cancel', event => {
  if ($('#publish-emergency').disabled) event.preventDefault();
});
$('#edit-emergency').addEventListener('click', () => emergencyPreview.close());
$('#publish-emergency').addEventListener('click', async event => {
  if (!emergencyDraft || !emergencyPreview.open) return;
  const { id, body } = emergencyDraft;
  const publish = event.currentTarget;
  const edit = $('#edit-emergency');
  const error = $('#emergency-preview-error');
  error.hidden = true;
  publish.disabled = edit.disabled = true;
  try {
    await saveCuration('emergency', id, body);
  } catch (failure) {
    error.textContent = failure.message || 'Publikasi gagal.';
    error.hidden = false;
    return;
  } finally {
    publish.disabled = edit.disabled = false;
  }
  emergencyPreview.close();
  if (state.role === 'ADMIN') {
    await action(null, async () => {
      await loadCuration();
      status('Kontak darurat diterbitkan dan perubahan tercatat.');
    });
  }
});
$('#curation-form').addEventListener('submit', event => {
  event.preventDefault();
  const form = event.currentTarget;
  action($('#save-record'), async () => {
    const kind = state.curationKind;
    const record = state.editingRecord;
    const body = { reason: form.elements.reason.value.trim() };
    if (!body.reason) throw new Error('Isi alasan perubahan.');
    for (const [path, , type] of [...curationFields[kind], ...sourceFields]) {
      const [parent, child] = path.split('.');
      const original = fieldValue(record, path);
      const value = curationValue(form.elements[path], type, original);
      if (child) {
        body[parent] ??= { ...record?.[parent] };
        body[parent][child] = value;
      } else {
        body[parent] = value;
      }
    }
    const id = record && recordId(record, kind);
    if (kind === 'emergency') {
      showEmergencyPreview(id, body);
      return;
    }
    await saveCuration(kind, id, body);
    if (state.curationKind === kind) await loadCuration();
    status('Entri disimpan dengan jejak audit.');
  });
});
$('#delete-record').addEventListener('click', event => action(event.currentTarget, async () => {
  const reason = $('#curation-form').elements.reason.value.trim();
  if (!reason) throw new Error('Isi alasan perubahan.');
  const kind = state.curationKind;
  const id = recordId(state.editingRecord, kind);
  if (!confirm(kind === 'emergency' ? 'Hapus entri darurat ini?' : 'Cabut entri ini?')) return;
  await api('/' + kind + '/' + encodeURIComponent(id), { method: 'DELETE', body: JSON.stringify({ reason }) });
  if (state.curationKind === kind) await loadCuration();
  status('Perubahan tersimpan.');
}));
$('#import-form').addEventListener('submit', event => { event.preventDefault(); const form = event.currentTarget; action(form.querySelector('button'), async () => { const data = new FormData(form); const dryRun = form.elements.dry_run.checked; const kind = data.get('kind'); data.delete('kind'); data.delete('dry_run'); const result = await api('/import?kind=' + encodeURIComponent(kind) + '&dry_run=' + dryRun, { method: 'POST', body: data }); $('#import-result').textContent = JSON.stringify(result, null, 2); status(dryRun ? 'Validasi selesai. Belum ada data disimpan.' : 'Impor selesai.'); }); });
async function loadAudit() {
  const data = items(await api('/audit?limit=100'));
  const rows = data.map(item => { const row = node('tr'); row.append(node('td', date(item.at || item.created_at)), node('td', item.actor || item.actor_id), node('td', `${item.action} · ${item.target}`), node('td', item.reason)); return row; });
  if (!rows.length) { const row = node('tr'); const cell = node('td', 'Belum ada perubahan staf.'); cell.colSpan = 4; row.append(cell); rows.push(row); } $('#audit-rows').replaceChildren(...rows);
}
$('#load-audit').addEventListener('click', event => action(event.currentTarget, loadAudit));
$('#export-form').addEventListener('submit', event => { event.preventDefault(); const form = event.currentTarget; action(form.querySelector('button'), async () => { const query = new URLSearchParams(new FormData(form)); const response = await fetch('/api/v1/admin/export?' + query, { credentials: 'same-origin', cache: 'no-store' }); if (!response.ok) { const body = await response.json().catch(() => ({})); throw new Error(body.message || 'Ekspor gagal.'); } const blob = await response.blob(); const url = URL.createObjectURL(blob); const link = node('a'); link.href = url; link.download = `calltag-${query.get('dataset')}.${query.get('format')}`; document.body.append(link); link.click(); link.remove(); setTimeout(() => URL.revokeObjectURL(url), 1000); status('Rekap siap diunduh.'); }); });
restoreSession();
