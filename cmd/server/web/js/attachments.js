/* ============================================================
   attachments.js — 工单 / 事件评论 / AI 对话共用的附件准备与渲染
   ============================================================ */
const ATTACH_FILE_ACCEPT = "image/*,.txt,.log,.conf,.cfg,.ini,.yaml,.yml,.json,.md,.sh,.py,.go,.js,.ts,.sql,.xml,.csv,.toml,.properties,.env,.docx,.xlsx,.pdf";
const ATTACH_TEXT_EXTS = /\.(txt|log|conf|cfg|ini|yaml|yml|json|md|sh|py|go|js|ts|sql|xml|csv|toml|properties|env)$/i;
const ATTACH_PARSE_EXTS = /\.(docx|xlsx|pdf)$/i;

function attachChipsHTML(atts, opts){
  opts = opts || {};
  if (!atts || !atts.length) return "";
  const rem = opts.removable ? true : false;
  return `<div class="attach-chips">${atts.map((a,i)=>{
    const name = esc(a.name || (a.kind==="image"?"image":"file"));
    if (a.kind==="image" && a.data) {
      const mime = esc(a.mime || "image/png");
      return `<a class="attach-chip img" href="data:${mime};base64,${a.data}" target="_blank" rel="noopener" title="${name}">🖼 ${name}${rem?`<button type="button" data-att-rm="${i}" aria-label="移除">✕</button>`:""}</a>`;
    }
    const tip = esc(String(a.text||"").slice(0,240));
    return `<span class="attach-chip file" title="${tip}">📄 ${name}${rem?`<button type="button" data-att-rm="${i}" aria-label="移除">✕</button>`:""}</span>`;
  }).join("")}</div>`;
}

function renderAttachBox(el, atts, onRemove){
  if (!el) return;
  if (!atts || !atts.length){ el.innerHTML=""; el.style.display="none"; return; }
  el.style.display="flex";
  el.innerHTML = attachChipsHTML(atts, {removable: !!onRemove});
  if (onRemove) el.querySelectorAll("[data-att-rm]").forEach(b=>{
    b.onclick = e => { e.preventDefault(); e.stopPropagation(); onRemove(parseInt(b.dataset.attRm,10)); };
  });
}

function attachmentsToAPI(atts){
  return (atts||[]).filter(a=>a && (a.data || a.text)).map(a=>({
    name: a.name||"", mime: a.mime||"", kind: a.kind|| (a.data?"image":"file"),
    data: a.kind==="image" ? (a.data||"") : "",
    text: a.kind!=="image" ? (a.text||"") : ""
  }));
}

function attachmentsToAIPayload(atts){
  const images=[], files=[];
  (atts||[]).forEach(a=>{
    if (a.kind==="image" && a.data) images.push({mime:a.mime||"image/png", data:a.data});
    else if (a.text) files.push({name:a.name||"file", text:a.text});
  });
  return {images, files};
}

async function fileToBase64(file){
  const buf = await file.arrayBuffer();
  const bytes = new Uint8Array(buf);
  let bin=""; const chunk=0x8000;
  for (let i=0;i<bytes.length;i+=chunk) bin += String.fromCharCode.apply(null, bytes.subarray(i,i+chunk));
  return btoa(bin);
}

async function ingestFilesIntoAttachments(fileList, targetArr, opts){
  opts = opts || {};
  const maxImg = opts.maxImages || 4;
  const list = Array.from(fileList||[]);
  for (const f of list){
    if (f.type && f.type.startsWith("image/")){
      if (targetArr.filter(a=>a.kind==="image").length >= maxImg){
        if (typeof toast==="function") toast(I18N.t("sre.max_4_images","最多 4 张图片"),"err");
        continue;
      }
      await new Promise((resolve,reject)=>{
        const rd=new FileReader();
        rd.onload=()=>{ const s=String(rd.result||""); const c=s.indexOf(","); targetArr.push({kind:"image",name:f.name,mime:f.type||"image/png",data:c>=0?s.slice(c+1):s}); resolve(); };
        rd.onerror=reject; rd.readAsDataURL(f);
      });
      if (opts.onChange) opts.onChange();
      continue;
    }
    if (ATTACH_TEXT_EXTS.test(f.name) || (f.type&&f.type.startsWith("text/"))){
      await new Promise((resolve,reject)=>{
        const rd=new FileReader();
        rd.onload=()=>{ targetArr.push({kind:"file",name:f.name,mime:f.type||"text/plain",text:String(rd.result||"")}); resolve(); };
        rd.onerror=reject; rd.readAsText(f);
      });
      if (opts.onChange) opts.onChange();
      continue;
    }
    if (ATTACH_PARSE_EXTS.test(f.name)){
      const ph={kind:"file",name:f.name+"（解析中…）",text:"",_parsing:true};
      targetArr.push(ph); if (opts.onChange) opts.onChange();
      try {
        const b64 = await fileToBase64(f);
        const r = await fetch(`${API}/hermes/parse`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({name:f.name,mime:f.type||"",data:b64})});
        const j = await r.json().catch(()=>({}));
        const idx = targetArr.indexOf(ph);
        if (!r.ok || j.error){
          if (idx>=0) targetArr.splice(idx,1);
          if (typeof toast==="function") toast(`${I18N.t("sre.parse_v","解析")} ${f.name} ${I18N.t("sre.failed_v","失败")}：${(j&&j.error)||r.status}`,"err");
        } else if (idx>=0){
          targetArr[idx]={kind:"file",name:f.name,mime:f.type||"",text:j.text||j.content||""};
        }
      } catch(e){
        const idx = targetArr.indexOf(ph);
        if (idx>=0) targetArr.splice(idx,1);
        if (typeof toast==="function") toast(`${I18N.t("sre.parse_v","解析")} ${f.name} ${I18N.t("sre.failed_v","失败")}`,"err");
      }
      if (opts.onChange) opts.onChange();
      continue;
    }
    if (typeof toast==="function") toast(`${I18N.t("sre.unsupported_file","不支持的文件类型")}：${f.name}`,"err");
  }
}

/* ---- 用户目录（工单指派） ---- */
let DIRECTORY_USERS = null;
async function loadDirectoryUsers(force){
  if (DIRECTORY_USERS && !force) return DIRECTORY_USERS;
  try {
    const r = await fetch(`${API}/directory/users`);
    DIRECTORY_USERS = (await r.json().catch(()=>[])) || [];
  } catch(e){ DIRECTORY_USERS = []; }
  return DIRECTORY_USERS;
}
async function fillUserSelect(sel, selected){
  if (!sel) return;
  const users = await loadDirectoryUsers();
  const cur = selected || "";
  const opts = [`<option value="">${I18N.t("ticket.unassigned","未指派")}</option>`]
    .concat(users.map(u=>{
      const v = esc(u.username||"");
      const label = esc(u.label || u.display_name || u.username || "");
      return `<option value="${v}"${u.username===cur?" selected":""}>${label}</option>`;
    }));
  if (cur && !users.some(u=>u.username===cur)) {
    opts.push(`<option value="${esc(cur)}" selected>${esc(cur)}（历史）</option>`);
  }
  sel.innerHTML = opts.join("");
  if (cur) sel.value = cur;
}
