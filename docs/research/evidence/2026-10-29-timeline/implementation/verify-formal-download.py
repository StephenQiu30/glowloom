import argparse,hashlib,json,subprocess
from pathlib import Path
import numpy as np
parser=argparse.ArgumentParser(description="Read-only decoded sampling for the synthetic 4s video + 5s image / 7s audio fixture")
parser.add_argument("download",type=Path)
parser.add_argument("samples",type=Path)
parser.add_argument("output",type=Path)
args=parser.parse_args()
FILE,SAMPLES,OUT=args.download,args.samples,args.output
def run(args):
 p=subprocess.run(args,check=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 if p.stderr: print(p.stderr.decode())
 return p.stdout
def probe(p):
 d=json.loads(run(['ffprobe','-v','error','-show_streams','-show_format','-of','json',str(p)])); d['format']['filename']=p.name
 return d
def audio(p):return np.frombuffer(run(['ffmpeg','-v','error','-i',str(p),'-map','0:a:0','-ac','1','-ar','48000','-f','f32le','pipe:1']),'<f4')
def frames(p,filters):return np.frombuffer(run(['ffmpeg','-v','error','-i',str(p),'-an','-vf',filters,'-pix_fmt','rgb24','-f','rawvideo','pipe:1']),np.uint8).reshape(-1,90,160,3)
def window(a,t,n=.2):
 v=a[round(t*48000):round((t+n)*48000)].astype(float);w=np.hanning(len(v));amp=2*np.abs(np.fft.rfft(v*w))/sum(w); freqs=np.fft.rfftfreq(len(v),1/48000)
 return {'start_s':t,'duration_s':n,'fft_resolution_hz':48000/len(v),'measured_bin_for_440_hz':float(freqs[np.argmin(abs(freqs-440))]),'measured_bin_for_660_hz':float(freqs[np.argmin(abs(freqs-660))]),'rms':float(np.sqrt(np.mean(v*v))),'amplitude_440_hz':float(amp[np.argmin(abs(freqs-440))]),'amplitude_660_hz':float(amp[np.argmin(abs(freqs-660))]),'dominant_hz':float(freqs[np.argmax(amp)])}
video=SAMPLES/'timeline-test-4s.mp4';wav=SAMPLES/'timeline-test-7s.wav';pic=SAMPLES/'timeline-test-image.png'
a=audio(FILE); srcv=audio(video); srca=audio(wav)
v=frames(FILE,'scale=160:90');sv=frames(video,'fps=30,scale=160:90');im=frames(pic,'scale=160:90')[0]
visual=[{'frame':i,'time_s':round(i/30,6),'mae_to_still':float(np.mean(abs(v[i].astype(float)-im.astype(float)))),'rgb_mean':v[i].mean(axis=(0,1)).tolist()} for i in [0,30,118,119,120,121,150,209,269]]
mae_source=np.mean(abs(v[:120].astype(float)-sv[:120].astype(float)),axis=(1,2,3));mae_image=np.mean(abs(v[120:].astype(float)-im.astype(float)),axis=(1,2,3))
windows=[window(a,t) for t in [.2,.75,.9,1,1.1,2,3.75,4.05,4.5,6.75,7.05,8.5]]
bounds={str(t):[window(a,round(t+off,3),.02) for off in [-.04,-.02,0,.02,.04]] for t in [1,4,7]}
rms=np.sqrt(np.mean(a[:7*48000].reshape(-1,480).astype(float)**2,axis=1))
# 10ms blocks detect missing mixed output; per-tone 100ms Hann FFT scans detect lost layers.
scans={}
for hz,end in [(440,4),(660,7)]:
 vals=[]
 for t in np.arange(.05,end-.1,.01):
  d=window(a,float(t),.1); vals.append((float(t),d[f'amplitude_{hz}_hz']))
 scans[str(hz)]={'window_s':.1,'step_s':.01,'range_s':[.05,end-.05],'min_amplitude':min(x[1] for x in vals),'max_amplitude':max(x[1] for x in vals),'below_0_01_windows':[x[0] for x in vals if x[1]<.01]}
result={'file':FILE.name,'job_id':'cf82b065-b2cf-4ae8-9e3b-dfe8d64ba8bb','attempt':2,'sha256':hashlib.sha256(FILE.read_bytes()).hexdigest(),'bytes':FILE.stat().st_size,'probe':probe(FILE),'source_files':[{'name':p.name,'sha256':hashlib.sha256(p.read_bytes()).hexdigest(),'probe':probe(p)} for p in [video,wav,pic]],'source_audio_fft':[{'source':video.name,**window(srcv,.5)}, {'source':wav.name,**window(srca,.5)}],'audio_windows':windows,'boundary_20ms_windows':bounds,'audio_continuity':{'block_s':.01,'range_s':[0,7],'minimum_rms':float(min(rms)),'minimum_rms_start_s':float(np.argmin(rms)*.01),'blocks_below_rms_0_001':np.where(rms<.001)[0].tolist(),'tone_scan':scans,'tail_after_7_05_rms':float(np.sqrt(np.mean(a[round(7.05*48000):9*48000].astype(float)**2))),'decoded_samples':len(a),'last_nonzero_after_7_s':7+(int(np.flatnonzero(a[7*48000:] != 0)[-1])+1)/48000,'max_abs_7_to_7_02_s':float(np.max(np.abs(a[7*48000:round(7.02*48000)])))},'video_comparison':{'decoded_frame_count':len(v),'source_resampled_frame_count':len(sv),'video_first_120_frames_mae_mean':float(mae_source.mean()),'video_first_120_frames_mae_max':float(mae_source.max()),'still_last_150_frames_mae_mean':float(mae_image.mean()),'still_last_150_frames_mae_max':float(mae_image.max()),'frames':visual},'decode_stderr':'empty; full video and audio streams decoded successfully'}
OUT.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
print(json.dumps({k:result[k] for k in ['sha256','bytes','source_audio_fft','audio_windows','boundary_20ms_windows','audio_continuity','video_comparison']},ensure_ascii=False,indent=2))
