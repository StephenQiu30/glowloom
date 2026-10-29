from pathlib import Path
exec((Path(__file__).resolve().parent/'analyze-source-frames.py').read_text().split('rows=[]')[0])
frames=json.load(open(r/'source-frame-times.json'));times=[(x['before']+x['after'])/2 for x in frames];signals=[]
# Visible end section avoids the foreground director card and menu; measure red against blue in a 9x9 neighborhood.
for ti in frames:
 im=Image.open(r/'source-frames'/('%03d.png'%ti['index'])).convert('RGB');v=[]
 for k in range(990,1440,10):
  x,y=xy[k]; xx=round((x*z+m['e']-695)*2);yy=round((y*z+m['f']-212)*2)
  colors=[im.getpixel((xx+dx,yy+dy)) for dx in range(-4,5) for dy in range(-4,5)]
  v.append(max(c[0]-c[2] for c in colors))
 signals.append(v)
def score(period):
 err=[]
 for i,t in enumerate(times):
  target=t+period;j=bisect.bisect_left(times,target)
  if j>=len(times) or j==0:continue
  f=(target-times[j-1])/(times[j]-times[j-1]);err.extend((a-(b+f*(c-b)))**2 for a,b,c in zip(signals[i],signals[j-1],signals[j]))
 return sum(err)/len(err)
scores=[(score(p),p) for p in range(2400,2801)];best=min(scores);out={'estimateMs':best[1],'mse':best[0],'exact2600Mse':score(2600),'samplingIntervalMeanMs':(times[-1]-times[0])/(len(times)-1),'timestampWindowMaxMs':max(x['after']-x['before'] for x in frames),'method':'Phase-shift fit of red-minus-blue maxima in 45 visible curve neighborhoods; linearly interpolate irregular screenshot times; occluded sections excluded. Static bundle sets exact period; image fit is independent coarse corroboration.','scoresEvery10Ms':[{ 'period':p,'mse':e} for e,p in scores if p%10==0]}
json.dump(out,open(r/'source-period-fit.json','w'),indent=2);print(json.dumps(out,ensure_ascii=False)[:1500])
