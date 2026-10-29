"""Extract CC0 Standard rotation tracks; source path is explicit, no network access."""
import json, struct, sys, hashlib
from pathlib import Path
src, dest = map(Path, sys.argv[1:3])
b=src.read_bytes(); n=struct.unpack_from('<I',b,12)[0]; doc=json.loads(b[20:20+n]); off=20+n; binary=b[off+8:]
def accessor(i):
 a=doc['accessors'][i]; v=doc['bufferViews'][a['bufferView']]
 if a['componentType'] != 5126 or 'sparse' in a: raise ValueError('unsupported accessor')
 width={'SCALAR':1,'VEC4':4}[a['type']]; stride=v.get('byteStride',4*width); start=v.get('byteOffset',0)+a.get('byteOffset',0)
 return [x for row in range(a['count']) for x in struct.unpack_from('<'+'f'*width,binary,start+row*stride)]
rest={}; motions=[]
for anim in doc['animations']:
 tracks=[]; duration=0
 for channel in anim['channels']:
  if channel['target']['path']!='rotation': continue
  node=doc['nodes'][channel['target']['node']]; name=node['name']; rest[name]=node.get('rotation',[0,0,0,1]); sampler=anim['samplers'][channel['sampler']]
  if sampler.get('interpolation','LINEAR')!='LINEAR': raise ValueError('unsupported interpolation')
  times=accessor(sampler['input']); values=accessor(sampler['output']); duration=max(duration,max(times))
  tracks.append({'bone':name,'times':times,'values':values})
 motions.append({'name':anim['name'],'duration':duration,'tracks':tracks})
assert len(motions)==43 and len(rest)==65
out=json.dumps({'version':1,'source':{'license':'CC0-1.0','author':'Quaternius','url':'https://quaternius.com/packs/universalanimationlibrary.html','sha256':hashlib.sha256(b).hexdigest()},'rest_rotations':rest,'motions':motions},separators=(',',':'))
dest.write_text(out)
print(json.dumps({'source_sha256':hashlib.sha256(b).hexdigest(),'motions':len(motions),'bytes':len(out),'output_sha256':hashlib.sha256(out.encode()).hexdigest()}))
