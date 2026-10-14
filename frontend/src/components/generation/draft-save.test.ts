import { expect, it, vi } from "vitest";
import { DraftSaver, type LocalBranch, type SavedDraft } from "./draft-save";
import { emptyPromptDraft } from "./prompt-document";
const snapshot=(seq:number)=>({contextKey:"user/project/draft",replicaId:"a",localSeq:seq,draft:{...emptyPromptDraft(),params:{seed:seq}}});
it("single flight acknowledges only captured input and drains the latest edit",async()=>{
 let release!:(value:SavedDraft)=>void;
 const send=vi.fn().mockImplementationOnce(()=>new Promise< SavedDraft >(r=>release=r)).mockImplementationOnce(async(r)=>({draft_id:"d",revision:2,draft_hash:"hash2",input_hash:"input2",draft:r.snapshot.draft}));
 const persisted:LocalBranch[]=[];
 const saver=new DraftSaver({snapshot:snapshot(1),revision:0,send,persist:async b=>{persisted.push(structuredClone(b))}});
 const first=saver.flush();await vi.waitFor(()=>expect(send).toHaveBeenCalledTimes(1));
 saver.edit(snapshot(2));const second=saver.flush();expect(send).toHaveBeenCalledTimes(1);
 release({draft_id:"d",revision:1,draft_hash:"hash",input_hash:"input",draft:snapshot(1).draft});
 await Promise.all([first,second]);expect(send).toHaveBeenCalledTimes(2);expect(send.mock.calls[1][0].expectedRevision).toBe(1);expect(saver.branch.snapshot.localSeq).toBe(2);expect(saver.branch.ackedSeq).toBe(2);expect(persisted.some(b=>!!b.pendingSave)).toBe(true);
});
it("unknown result retries exactly the frozen key/body before newer edits",async()=>{
 const send=vi.fn().mockRejectedValueOnce(new Error("offline")).mockImplementation(async r=>({draft_id:"d",revision:r.expectedRevision+1,draft_hash:"hash",input_hash:"input",draft:r.snapshot.draft}));
 const saver=new DraftSaver({snapshot:snapshot(1),revision:0,send,persist:async()=>{}});
 await expect(saver.flush()).rejects.toThrow();saver.edit(snapshot(2));await saver.flush();
 expect(send.mock.calls[0][0]).toEqual(send.mock.calls[1][0]);expect(send.mock.calls[2][0].key).not.toBe(send.mock.calls[0][0].key);expect(saver.branch.ackedSeq).toBe(2);
});
