import { NextRequest, NextResponse } from "next/server";
import { COOKIE_NAME, isSessionValid, liveConfig } from "@/lib/server/live-auth";
export const runtime = "nodejs";
export const dynamic = "force-dynamic";
const headers = { "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff" };
export async function GET(req: NextRequest) {
 const config=liveConfig();
 if (!config) return NextResponse.json({error:"backend unavailable"},{status:503,headers});
 if (!isSessionValid(req.cookies.get(COOKIE_NAME)?.value,config)) return NextResponse.json({error:"authentication required"},{status:401,headers});
 const id=req.nextUrl.searchParams.get("session_id") ?? "";
 if (!/^[a-f0-9]{32}$/.test(id)) return NextResponse.json({error:"invalid session"},{status:400,headers});
 try {
  const upstream=await fetch(`${config.baseURL}/api/v1/conversation/sessions/${id}`,{headers:{Authorization:`Bearer ${config.apiToken}`},cache:"no-store",redirect:"manual",signal:req.signal});
  if(!upstream.ok)return NextResponse.json({error:"session unavailable"},{status:upstream.status===404?404:503,headers});
  const item:unknown=await upstream.json();
  if(!item||typeof item!=="object")return NextResponse.json({error:"invalid session response"},{status:502,headers});
  const ref=item as Record<string,unknown>;
  if(ref.session_id!==id||typeof ref.run_id!=="string"||!/^[a-f0-9]{32}$/.test(ref.run_id)||typeof ref.symbol!=="string"||typeof ref.as_of!=="string")
   return NextResponse.json({error:"invalid session reference"},{status:502,headers});
  return NextResponse.json({session_id:id,run_id:ref.run_id,symbol:ref.symbol,as_of:ref.as_of},{headers});
 } catch {return NextResponse.json({error:"session unavailable"},{status:503,headers});}
}
