import subprocess,json,time
from pathlib import Path
base=['agent-browser','--session','glowloom-header-final']
def run(*args):return subprocess.check_output(base+list(args),text=True,timeout=45)
def measure():
 s=run('eval', '''JSON.stringify({viewport:innerWidth,width:document.documentElement.scrollWidth,controls:[...document.querySelectorAll('header button')].map(e=>({name:e.getAttribute('aria-label')||e.innerText,x:e.getBoundingClientRect().x,y:e.getBoundingClientRect().y,w:e.offsetWidth,h:e.offsetHeight})),images:[...document.querySelectorAll('main img')].map(e=>({loaded:e.complete&&e.naturalWidth>0,width:e.naturalWidth,height:e.naturalHeight}))})''')
 return json.loads(json.loads(s))
results=[]
for width in [1440,1024,768,390,320]:
 run('set','viewport',str(width),'900');time.sleep(.35)
 m=measure();m['mode']='normal';assert m['width']==width,m
 if width<768:assert all(c['h']>=44 for c in m['controls']),m
 if width>=1024:assert len({c['y'] for c in m['controls']})==1,m
 results.append(m)
 run('find','role','button','click','--name','批量操作');run('wait','[aria-label="主体批量操作"]');time.sleep(.1)
 m=measure();m['mode']='batch';assert m['width']==width,m
 assert not any(c['name']=='账号菜单' for c in m['controls']),m
 results.append(m)
 run('find','role','button','click','--name','取消选择');time.sleep(.1)
run('set','viewport','1440','900')
p=(Path(__file__).resolve().parent/'responsive.json');p.write_text(json.dumps(results,ensure_ascii=False,indent=2))
print('PASS: 10 responsive states; no horizontal overflow; desktop same row; mobile 44px controls; batch public actions removed')
