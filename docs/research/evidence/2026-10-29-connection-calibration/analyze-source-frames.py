import json,re,math,bisect
from PIL import Image
from pathlib import Path
r=Path(__file__).resolve().parent;g=json.load(open(r/'source-geometry.json')); nums=list(map(float,re.findall(r'[-+]?\d*\.?\d+',g['path'])));p=list(zip(nums[::2],nums[1::2]));pts=[];d=[0]
for i in range(65):
 t=i/64; pts.append(tuple((1-t)**3*p[0][a]+3*(1-t)**2*t*p[1][a]+3*(1-t)*t*t*p[2][a]+t**3*p[3][a] for a in (0,1)))
 if i:d.append(d[-1]+math.dist(pts[-1],pts[-2]))
m=g['matrix'];length=d[-1];z=m['a'];dis=[i*length/1499 for i in range(1500)];xy=[]
for v in dis:
 i=min(64,max(1,bisect.bisect_left(d,v)));f=(v-d[i-1])/(d[i]-d[i-1]);xy.append(tuple(pts[i-1][a]+f*(pts[i][a]-pts[i-1][a]) for a in (0,1)))
rows=[]
for ti in json.load(open(r/'source-frame-times.json')):
 im=Image.open(r/'source-frames'/('%03d.png'%ti['index'])).convert('RGB');rgb=[im.getpixel((min(im.width-1,max(0,round((x*z+m['e']-695)*2))),min(im.height-1,max(0,round((y*z+m['f']-212)*2))))) for x,y in xy];strength=[max(0,c[0]-c[2]) for c in rgb];good=[i for i,s in enumerate(strength) if s>20];peak=max(range(len(strength)),key=strength.__getitem__)
 rows.append({**ti,'time':(ti['before']+ti['after'])/2,'orangeCount':len(good),'orangeMin':dis[good[0]] if good else None,'orangeMax':dis[good[-1]] if good else None,'peak':dis[peak],'maxRed':max(c[0] for c in rgb),'rgbAtPeak':rgb[peak]})
json.dump({'length':length,'tailLength':min(312,max(120,.55*length)),'zoom':z,'rows':rows},open(r/'source-pixel-samples.json','w'),indent=2)
print('L',length,'S',min(312,max(120,.55*length)));print('\n'.join(str({k:v for k,v in x.items() if k in ['index','time','orangeCount','peak','rgbAtPeak']}) for x in rows))
