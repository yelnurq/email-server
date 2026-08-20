"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { API_URL, api } from "@/lib/api";

type Notification={id:string;kind:string;title:string;body:string;target_url:string;read_at?:string;created_at:string};

export function NotificationsBell(){
 const [open,setOpen]=useState(false),qc=useQueryClient();
 const query=useQuery({queryKey:["notifications"],queryFn:()=>api.get<{notifications:Notification[];unread:number}>("/api/v1/notifications")});
 useEffect(()=>{const path=`${API_URL}/api/v1/realtime`,url=API_URL.startsWith("http")?new URL(path):new URL(path,window.location.href);url.protocol=url.protocol==="https:"?"wss:":"ws:";const ws=new WebSocket(url);ws.onmessage=()=>qc.invalidateQueries({queryKey:["notifications"]});return()=>ws.close()},[qc]);
 async function read(item:Notification){if(!item.read_at){await api.post(`/api/v1/notifications/${item.id}/read`);qc.invalidateQueries({queryKey:["notifications"]})}setOpen(false)}
 return <div className="relative"><button type="button" onClick={()=>setOpen(v=>!v)} className="relative grid h-9 w-9 place-items-center rounded-lg text-muted-foreground hover:bg-muted" aria-label="Notifications"><svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8"><path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4"/></svg>{Boolean(query.data?.unread)&&<span className="absolute right-1 top-1 min-w-4 rounded-full bg-danger px-1 text-center text-[9px] font-bold leading-4 text-white">{query.data!.unread>99?"99+":query.data!.unread}</span>}</button>{open&&<div className="absolute right-0 z-50 mt-2 w-[min(90vw,24rem)] overflow-hidden rounded-xl border border-border bg-surface-elevated shadow-[var(--shadow-popover)]"><div className="flex items-center justify-between border-b border-border px-4 py-3"><h2 className="text-sm font-semibold">Notifications</h2><button className="text-xs font-medium text-primary" onClick={async()=>{await api.post("/api/v1/notifications/read-all");qc.invalidateQueries({queryKey:["notifications"]})}}>Mark all read</button></div><div className="max-h-96 overflow-y-auto">{query.data?.notifications.length===0&&<p className="p-6 text-center text-sm text-muted-foreground">No notifications</p>}{query.data?.notifications.map(item=><Link key={item.id} href={item.target_url||"#"} onClick={()=>void read(item)} className={`block border-b border-border px-4 py-3 last:border-0 hover:bg-muted ${item.read_at?"":"bg-primary/5"}`}><div className="flex gap-2"><span className={`mt-1 h-2 w-2 shrink-0 rounded-full ${item.read_at?"bg-transparent":"bg-primary"}`}/><span className="min-w-0"><span className="block truncate text-sm font-semibold">{item.title}</span><span className="mt-0.5 block line-clamp-2 text-xs text-muted-foreground">{item.body}</span><span className="mt-1 block text-[10px] text-muted-foreground">{new Date(item.created_at).toLocaleString()}</span></span></div></Link>)}</div></div>}</div>
}
