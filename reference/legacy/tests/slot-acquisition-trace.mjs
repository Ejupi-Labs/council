/** Test-only bounded error phase metadata; never records SQL, run IDs or payloads. */
export function traceSlotAcquisition(store, origin) {
  const acquire=store.acquireSlot, records=[];let omitted=0;
  store.acquireSlot=function(...args){
    try{return acquire.apply(this,args);}
    catch(error){
      const code=error&&typeof error==='object'&&Number.isInteger(error.errcode)&&error.errcode>=0&&error.errcode<=0x7fffffff?error.errcode:undefined;
      const proof=origin(error), record={area:'state',operation:'acquire_slot',phase:proof?.area==='state'&&proof.phase==='begin'&&proof.code===code?'begin':'operation',...(code!==undefined?{sqliteCode:code}:{})};
      if(records.length<8)records.push(record);else omitted++;
      throw error;
    }
  };
  return ()=>({records:records.map(record=>({...record})),omitted});
}
