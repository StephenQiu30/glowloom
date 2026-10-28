from pathlib import Path
import re,json,subprocess,datetime
base=Path.cwd(); evidence=base/'docs/research/evidence/2026-10-28-jimeng-explore'
files=[base/x for x in ['docs/design/28-探索发现与创作转化设计.md','docs/prd/15-探索发现与创作转化.md','docs/plan/24-探索一致性改造Agent执行计划.md','docs/requirement/15-探索对齐即梦需求.md','docs/research/11-即梦探索完整需求分析.md']]
d,p,l,q,r=[f.read_text() for f in files];checks={}
def check(name,condition,detail):
    checks[name]={'passed':bool(condition),'detail':detail}
    assert condition,(name,detail)
headings=re.findall(r'^## (\d+)\. (.+)$',d,re.M)
check('design_eight_sections',[x[1] for x in headings]==['摘要','参与者','背景','目标','使用场景','价值','方案','实施与验收'],headings)
source=set(re.findall(r'^\| (EX-(?:N?FR)-\d\d)\s*\|',r,re.M)); mapped=[]
for line in p.split('### 7.4 ')[1].split('### 7.5 ')[0].splitlines():
    if not line.startswith('| EX-'):continue
    for prefix,start,end in re.findall(r'(EX-(?:N?FR)-)(\d\d)(?:～(\d\d))?',line.split('|')[1]):
        mapped += [f'{prefix}{n:02}' for n in range(int(start),int(end or start)+1)]
check('source_coverage',len(source)==50 and len(mapped)==50 and len(set(mapped))==50 and set(mapped)==source,{'source':len(source),'mapped':len(mapped),'missing':sorted(source-set(mapped))})
for name,content,pat,total in [('requirements',q,r'^\| EX-REQ-(\d\d)\s*\|',7),('stories',p,r'^\| ST-EX-(\d\d)：',10),('acceptance',p,r'^\| EX-AC-(\d\d)\s*\|',22),('cases',l,r'^\| CASE-EX-(\d\d) /',22),('tasks',l,r'^\| T-EX-(\d\d) ',15),('figma_sets',d,r'^\| FIG-EX-(\d\d) ',10),('decisions',d,r'^\| D(\d\d) ',8)]:
    ids=re.findall(pat,content,re.M); start=0 if name=='tasks' else 1
    check(name,len(ids)==total and set(ids)=={f'{n:02}' for n in range(start,start+total)},ids)
check('single_source_prd',p.count('唯一来源设计：')==1 and '../design/28-' in p.split('\n')[2], 'PRD15 -> design28')
check('single_source_plan',l.count('唯一来源 PRD：')==1 and '../prd/15-' in l.split('\n')[2], 'Plan24 -> PRD15')
edges=re.findall(r'\b(T\d\d)(?:\[[^\]]+\])? --> (T\d\d)',l); nodes={f'T{n:02}' for n in range(15)};remaining=set(nodes);ordered=[]
while remaining:
    ready=sorted(n for n in remaining if not any(b==n and a in remaining for a,b in edges))
    assert ready,'dependency cycle'
    ordered.extend(ready);remaining.difference_update(ready)
check('dependency_graph',all(a in nodes and b in nodes for a,b in edges) and len(edges)>=20,{'edges':len(edges),'topological_order':ordered})
shared=[base/x for x in ['DESIGN.md','BACKLOG.md','docs/00-文档索引.md','docs/design/00-设计导航.md','docs/design/09-统一顶栏与阶段入口设计.md','docs/prd/00-PRD导航.md','docs/plan/00-Plan导航.md','docs/requirement/00-需求导航.md','docs/research/00-调研导航.md']]
report=evidence/'planning-validation.md';bad=[];warnings=[];count=0
for f in files+shared:
    for target in re.findall(r'\]\(([^)]+)\)',f.read_text()):
        if re.match(r'[a-zA-Z]+:|#',target):continue
        target=target.split('#')[0].split(' "')[0].strip('<>')
        if not target:continue
        count+=1; resolved=(f.parent/target).resolve()
        if not resolved.exists() and resolved!=report:
            item=[str(f.relative_to(base)),target]
            (bad if f in files or 'jimeng-explore' in target or '探索' in target else warnings).append(item)
check('task_local_links',not bad,{'checked_including_shared':count,'task_missing':bad,'unrelated_shared_missing':warnings,'anchors':'no new anchor links; existing shared anchors retained'})
result={'checkedAt':datetime.datetime.now().astimezone().isoformat(),'head':subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(),'checks':checks,'scope':'documentation only; no application, Figma, service, or product acceptance executed'}
(evidence/'planning-checks.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
print(json.dumps({'checks':len(checks),'allTaskChecksPassed':True,'links':count,'sourceCoverage':len(mapped),'unrelatedSharedMissing':warnings},ensure_ascii=False))
