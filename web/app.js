const $ = (s, root=document) => root.querySelector(s);
const $$ = (s, root=document) => [...root.querySelectorAll(s)];
let state = {connections:[], keys:[], version:''};
let histories = {cpu:[], memory:[], disk:[], rx:[], tx:[]};

const api = async (url, options={}) => {
  const o = {...options, headers:{'Content-Type':'application/json', ...(options.headers||{})}};
  const r = await fetch(url, o);
  let data = null; try { data = await r.json(); } catch {}
  if (r.status === 401) { showLogin(); throw new Error('unauthorized'); }
  if (!r.ok) throw new Error(data?.error || `HTTP ${r.status}`);
  return data;
};

function showLogin(){ $('#app').classList.add('hidden'); $('#loginView').classList.remove('hidden'); }
function showApp(){ $('#loginView').classList.add('hidden'); $('#app').classList.remove('hidden'); }
function toast(msg, error=false){ const t=$('#toast'); t.textContent=msg; t.className='toast'+(error?' error':''); setTimeout(()=>t.classList.add('hidden'),3200); }
function escapeHtml(s=''){ return String(s).replace(/[&<>'"]/g, c=>({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c])); }
function fmtDate(s){ if(!s)return '—'; const d=new Date(s); return d.toLocaleString('ru-RU',{day:'2-digit',month:'2-digit',year:'numeric',hour:'2-digit',minute:'2-digit'}); }
function bytes(n){ if(!Number.isFinite(n)) return '—'; const u=['B','KB','MB','GB','TB']; let i=0,v=n; while(v>=1024&&i<u.length-1){v/=1024;i++} return `${v.toFixed(v>=100?0:v>=10?1:2)} ${u[i]}`; }
function rate(n){ return bytes(n)+'/s'; }
function connName(id){ return state.connections.find(c=>c.id===id)?.name || '—'; }

$('#loginForm').addEventListener('submit', async e=>{
  e.preventDefault(); $('#loginError').textContent='';
  try { await api('/api/login',{method:'POST',body:JSON.stringify({Username:$('#loginUser').value,Password:$('#loginPass').value})}); await boot(); }
  catch(err){ if(err.message!=='unauthorized') $('#loginError').textContent=err.message; }
});

async function boot(){
  try{ state=await api('/api/state'); showApp(); render(); await refreshMetrics(); }
  catch(err){ if(err.message!=='unauthorized') toast(err.message,true); }
}

async function refreshState(){ state=await api('/api/state'); render(); }
function render(){
  $('#coreVersion').textContent=state.core_version||'неизвестна';
  $('#panelVersion').textContent=state.panel_version||state.version||'неизвестна';
  renderConnections(); renderKeys();
}

function renderConnections(){
  const body=$('#connectionsBody'); body.innerHTML=''; $('#connectionsEmpty').classList.toggle('hidden',state.connections.length>0);
  for(const c of state.connections){
    const tr=document.createElement('tr');
    tr.innerHTML=`<td><b>${escapeHtml(c.name)}</b></td><td>${c.port}</td><td>${escapeHtml(c.aead)}</td><td><code>${escapeHtml(c.table_type)}</code></td><td><span class="status ${c.active?'on':''}">${c.active?'Работает':'Остановлено'}</span></td><td><div class="actions"><button class="btn small" data-restart="${c.id}">↻</button><button class="btn small danger" data-delete-connection="${c.id}">🗑</button></div></td>`;
    body.appendChild(tr);
  }
}

function renderKeys(){
  const body=$('#keysBody'); body.innerHTML=''; $('#keysEmpty').classList.toggle('hidden',state.keys.length>0);
  for(const k of state.keys){
    const tr=document.createElement('tr');
    tr.innerHTML=`<td><b>${escapeHtml(k.Name||k.name)}</b></td><td>${escapeHtml(connName(k.ConnectionID||k.connection_id))}</td><td>${fmtDate(k.CreatedAt||k.created_at)}</td><td><div class="actions"><button class="btn small" data-link="${k.ID||k.id}">🔗 Ссылка</button><button class="btn small" data-qr="${k.ID||k.id}">▦ QR</button><button class="btn small" data-params="${k.ID||k.id}">⚙ Параметры</button><button class="btn small danger" data-delete-key="${k.ID||k.id}">🗑</button></div></td>`;
    body.appendChild(tr);
  }
}

function openModal(html, wide=false){ $('#modalContent').innerHTML=html; $('#modal').classList.toggle('wide',wide); $('#modalBackdrop').classList.remove('hidden'); }
function closeModal(){ $('#modalBackdrop').classList.add('hidden'); $('#modalContent').innerHTML=''; }
$('#modalClose').onclick=closeModal;
$('#modalBackdrop').addEventListener('click',e=>{ if(e.target===$('#modalBackdrop')) closeModal(); });

document.addEventListener('click', async e=>{
  const b=e.target.closest('[data-action],[data-restart],[data-delete-connection],[data-link],[data-qr],[data-params],[data-delete-key]'); if(!b)return;
  try{
    if(b.dataset.action==='new-connection') return connectionModal();
    if(b.dataset.action==='new-key') return keyModal();
    if(b.dataset.action==='logs') return logsModal();
    if(b.dataset.action==='logout'){ await api('/api/logout',{method:'POST'}); return showLogin(); }
    if(b.dataset.action==='update-core') return updateThing('/api/update/core','Sudoku');
    if(b.dataset.action==='update-panel') return updateThing('/api/update/panel','панель');
    if(b.dataset.restart){ await api(`/api/connections/${b.dataset.restart}/restart`,{method:'POST'}); toast('Sudoku перезапущен'); return refreshState(); }
    if(b.dataset.deleteConnection){ if(confirm('Удалить подключение и все его ключи?')){ await api(`/api/connections/${b.dataset.deleteConnection}`,{method:'DELETE'}); toast('Подключение удалено'); await refreshState(); } return; }
    if(b.dataset.deleteKey){ if(confirm('Удалить ключ из панели?')){ await api(`/api/keys/${b.dataset.deleteKey}`,{method:'DELETE'}); toast('Ключ удалён'); await refreshState(); } return; }
    if(b.dataset.link){ const x=await api(`/api/keys/${b.dataset.link}/link`); await navigator.clipboard.writeText(x.link); toast('Ссылка скопирована'); return; }
    if(b.dataset.params) return paramsModal(b.dataset.params);
    if(b.dataset.qr) return qrModal(b.dataset.qr);
  }catch(err){ toast(err.message,true); }
});

function connectionModal(){
  openModal(`<div class="modal-body"><h2>Создать подключение</h2><form id="connectionForm"><div class="form-grid">
    <div class="field"><label>Название</label><input name="name" value="Main" required></div>
    <div class="field"><label>Порт</label><input name="port" type="number" min="1" max="65535" value="9443" required></div>
    <div class="field"><label>Метод шифрования</label><select name="aead"><option>chacha20-poly1305</option><option>aes-128-gcm</option></select></div>
    <div class="field"><label>Тип таблицы</label><select name="table"><option>up_ascii_down_entropy</option><option>prefer_entropy</option><option>prefer_ascii</option><option>up_entropy_down_ascii</option></select></div>
    <div class="field"><label>Padding минимум</label><input name="pmin" type="number" value="2" min="0"></div>
    <div class="field"><label>Padding максимум</label><input name="pmax" type="number" value="7" min="0"></div>
    <div class="field full"><div class="switch-row"><label class="check"><input name="pure" type="checkbox"> Pure Downlink</label><label class="check"><input name="http" type="checkbox"> HTTP Mask</label></div></div>
  </div><div class="modal-footer"><button type="button" class="btn" id="cancelModal">Отмена</button><button class="btn primary" type="submit">Создать</button></div></form></div>`);
  $('#cancelModal').onclick=closeModal;
  $('#connectionForm').onsubmit=async e=>{ e.preventDefault(); const f=new FormData(e.target); try{ await api('/api/connections',{method:'POST',body:JSON.stringify({name:f.get('name'),port:+f.get('port'),AEAD:f.get('aead'),TableType:f.get('table'),PaddingMin:+f.get('pmin'),PaddingMax:+f.get('pmax'),PureDownlink:f.get('pure')==='on',HTTPMask:f.get('http')==='on'})}); closeModal(); toast('Подключение создано'); await refreshState(); }catch(err){toast(err.message,true)} };
}

function keyModal(){
  if(!state.connections.length) return toast('Сначала создай подключение',true);
  const opts=state.connections.map(c=>`<option value="${c.id}">${escapeHtml(c.name)} · ${c.port}</option>`).join('');
  openModal(`<div class="modal-body"><h2>Создать ключ доступа</h2><form id="keyForm"><div class="form-grid"><div class="field"><label>Название</label><input name="name" placeholder="iPhone" required></div><div class="field"><label>Подключение</label><select name="connection">${opts}</select></div></div><div class="modal-footer"><button type="button" class="btn" id="cancelModal">Отмена</button><button class="btn primary" type="submit">Создать ключ</button></div></form></div>`);
  $('#cancelModal').onclick=closeModal;
  $('#keyForm').onsubmit=async e=>{e.preventDefault();const f=new FormData(e.target);try{await api('/api/keys',{method:'POST',body:JSON.stringify({Name:f.get('name'),ConnectionID:f.get('connection')})});closeModal();toast('Ключ создан');await refreshState()}catch(err){toast(err.message,true)}};
}

async function paramsModal(id){
  const p=await api(`/api/keys/${id}/params`); const fields=[['IP-адрес сервера','server'],['Порт','port'],['Ключ / Пароль','key'],['Метод шифрования','aead'],['Тип таблицы','table_type'],['Пользовательская таблица','custom_table'],['Padding','padding'],['Pure Downlink','pure'],['HTTP Mask','http']];
  const values={...p,padding:`${p.padding_min} – ${p.padding_max}`,pure:p.pure_downlink?'Включено':'Выключено',http:p.http_mask?'Включено':'Выключено'};
  openModal(`<div class="modal-body"><h2>Параметры подключения</h2><div class="params-grid">${fields.map(([l,k])=>`<div class="param"><label>${l}</label><div class="param-box"><code>${escapeHtml(values[k]??'—')}</code><button class="btn small" data-copy-value="${escapeHtml(values[k]??'')}">⧉</button></div></div>`).join('')}</div><div class="modal-footer"><button class="btn" id="copyLinkFromParams">Копировать ссылку</button><button class="btn primary" id="closeParams">Готово</button></div></div>`);
  $$('[data-copy-value]').forEach(x=>x.onclick=async()=>{await navigator.clipboard.writeText(x.dataset.copyValue);toast('Скопировано')});
  $('#copyLinkFromParams').onclick=async()=>{const x=await api(`/api/keys/${id}/link`);await navigator.clipboard.writeText(x.link);toast('Ссылка скопирована')}; $('#closeParams').onclick=closeModal;
}

function qrModal(id){ openModal(`<div class="modal-body"><h2>QR-код</h2><div class="qr-wrap"><img src="/api/keys/${id}/qr" alt="QR"></div><div class="modal-footer"><button class="btn primary" id="closeQR">Готово</button></div></div>`); $('#closeQR').onclick=closeModal; }
async function logsModal(){ const x=await api('/api/logs'); openModal(`<div class="modal-body"><h2>Логи Sudoku</h2><pre class="logs">${escapeHtml(x.logs)}</pre><div class="modal-footer"><button class="btn" id="refreshLogs">Обновить</button><button class="btn primary" id="closeLogs">Закрыть</button></div></div>`,true); $('#closeLogs').onclick=closeModal; $('#refreshLogs').onclick=logsModal; }
async function updateThing(url,label){
  if(!confirm(`Проверить последнюю версию: ${label}?`))return;
  toast('Проверяю обновление…');
  try{
    const result=await api(url,{method:'POST'});
    toast(result.message||`${label}: обновление завершено`);
    if(result.updated){ setTimeout(()=>location.reload(),1200); }
    else { await refreshState(); }
  }catch(err){toast(err.message,true)}
}

async function refreshMetrics(){
  try{ const m=await api('/api/metrics'); updateMetric(m); }catch{}
}
function push(arr,v){arr.push(Number(v)||0);if(arr.length>45)arr.shift()}
function updateMetric(m){
  $('#cpuValue').textContent=m.cpu_percent.toFixed(1); $('#cpuMeta').textContent=`${m.cpu_cores} ядер`;
  $('#memoryValue').textContent=m.memory_percent.toFixed(1); $('#memoryMeta').textContent=`${bytes(m.memory_used)} / ${bytes(m.memory_total)}`;
  $('#diskValue').textContent=m.disk_percent.toFixed(1); $('#diskMeta').textContent=`${bytes(m.disk_used)} / ${bytes(m.disk_total)}`;
  $('#rxRate').textContent=rate(m.rx_rate); $('#txRate').textContent=rate(m.tx_rate); $('#rxTotal').textContent=`↓ ${bytes(m.rx_total)}`; $('#txTotal').textContent=`↑ ${bytes(m.tx_total)}`;
  push(histories.cpu,m.cpu_percent); push(histories.memory,m.memory_percent); push(histories.disk,m.disk_percent); push(histories.rx,m.rx_rate); push(histories.tx,m.tx_rate);
  drawSingle('cpuChart',histories.cpu,'#1677ff',100); drawSingle('memoryChart',histories.memory,'#7c3aed',100); drawSingle('diskChart',histories.disk,'#12b76a',100); drawTraffic();
}
function setupCanvas(id){ const c=document.getElementById(id),dpr=devicePixelRatio||1,r=c.getBoundingClientRect();c.width=Math.max(1,r.width*dpr);c.height=Math.max(1,r.height*dpr);const x=c.getContext('2d');x.setTransform(dpr,0,0,dpr,0,0);return {c,x,w:r.width,h:r.height}; }
function drawSingle(id,data,color,max=0){ if(!data.length)return;const {x,w,h}=setupCanvas(id);const M=max||Math.max(...data,1);x.clearRect(0,0,w,h);const pts=data.map((v,i)=>[i/(Math.max(data.length-1,1))*w,h-8-(v/M)*(h-18)]);const g=x.createLinearGradient(0,0,0,h);g.addColorStop(0,color+'38');g.addColorStop(1,color+'00');x.beginPath();x.moveTo(pts[0][0],h);pts.forEach(p=>x.lineTo(...p));x.lineTo(pts.at(-1)[0],h);x.closePath();x.fillStyle=g;x.fill();x.beginPath();pts.forEach((p,i)=>i?x.lineTo(...p):x.moveTo(...p));x.strokeStyle=color;x.lineWidth=2;x.stroke(); }
function drawTraffic(){const {x,w,h}=setupCanvas('trafficChart');const max=Math.max(...histories.rx,...histories.tx,1);const draw=(arr,color)=>{const pts=arr.map((v,i)=>[i/(Math.max(arr.length-1,1))*w,h-7-(v/max)*(h-15)]);x.beginPath();pts.forEach((p,i)=>i?x.lineTo(...p):x.moveTo(...p));x.strokeStyle=color;x.lineWidth=2;x.stroke()};x.clearRect(0,0,w,h);draw(histories.rx,'#3b82f6');draw(histories.tx,'#7c3aed');}

setInterval(refreshMetrics,2500); setInterval(()=>refreshState().catch(()=>{}),10000); window.addEventListener('resize',()=>{if(!$('#app').classList.contains('hidden')) refreshMetrics()});
boot();
