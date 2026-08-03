function e(e,t){let n=t.get?t.get(e):e?.[t.key];if(n==null)return``;if(typeof n==`object`)try{return JSON.stringify(n)}catch{return String(n)}return String(n)}function t(e){return/[",\n\r]/.test(e)?`"`+e.replace(/"/g,`""`)+`"`:e}function n(n,r){let i=r.map(e=>t(e.label)).join(`,`),a=n.map(n=>r.map(r=>t(e(n,r))).join(`,`)).join(`\r
`);return i+`\r
`+a}function r(e,t,n){let r=e instanceof Blob?e:new Blob([e],{type:n}),i=URL.createObjectURL(r),a=document.createElement(`a`);a.href=i,a.download=t,document.body.appendChild(a),a.click(),a.remove(),setTimeout(()=>URL.revokeObjectURL(i),1e3)}function i(e,t,i){let a=i.endsWith(`.csv`)?i:i+`.csv`;r(n(e,t),a,`text/csv;charset=utf-8`)}function a(e){return e.replace(/&/g,`&amp;`).replace(/</g,`&lt;`).replace(/>/g,`&gt;`).replace(/"/g,`&quot;`).replace(/'/g,`&apos;`)}function o(t,n,i){r(`<?xml version="1.0"?>
<?mso-application progid="Excel.Sheet"?>
<Workbook xmlns="urn:schemas-microsoft-com:office:spreadsheet" xmlns:ss="urn:schemas-microsoft-com:office:spreadsheet">
 <Worksheet ss:Name="Sheet1">
  <Table>
   <Row>${n.map(e=>`<Cell><Data ss:Type="String">${a(e.label)}</Data></Cell>`).join(``)}</Row>
   ${t.map(t=>`<Row>${n.map(n=>{let r=e(t,n);return`<Cell><Data ss:Type="${r!==``&&!isNaN(Number(r))?`Number`:`String`}">${a(r)}</Data></Cell>`}).join(``)}</Row>`).join(``)}
  </Table>
 </Worksheet>
</Workbook>`,i.endsWith(`.xls`)?i:i+`.xls`,`application/vnd.ms-excel`)}function s(e,t,n,r=`csv`){r===`xlsx`?o(e,t,n):i(e,t,n)}export{s as t};