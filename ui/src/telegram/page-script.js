(()=>{const f=document.querySelector('form.vp'),T=[...document.querySelectorAll('time[data-left]')],t0=Date.now();let s=0;
const c=n=>{n=Math.max(0,Math.ceil(n));const h=n/3600|0,m=n%3600/60|0,r=n%60,p=x=>String(x).padStart(2,'0');return(h?h+':'+p(m):m)+':'+p(r)};
f&&f.addEventListener('change',()=>{s=1;const a=f.querySelector('.auto');if(a)a.textContent='Timer stopped. Pick when ready.'});
setInterval(()=>{const e=(Date.now()-t0)/1e3;for(const t of T){const l=t.dataset.left-e;if(l>=3600)continue;t.textContent='in '+c(l);
if(l<=0&&!s&&f&&f.contains(t)){s=1;t.textContent='picking now';f.requestSubmit(f.querySelector('[value=pick]'))}}},1e3)})();
