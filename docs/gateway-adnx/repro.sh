#!/usr/bin/env bash
# Tai hien loi tool_reference cua gateway ai.adnx.vn.
# Can: bash, curl, jq. Key lay tu bien GATEWAY_KEY (hoac ~/.wj/profiles/adnx.json neu co).
# Goi THANG gateway, khong qua proxy (proxy wj se sua request va che mat loi).
unset HTTPS_PROXY HTTP_PROXY https_proxy http_proxy ALL_PROXY all_proxy
K="${GATEWAY_KEY:-}"
[ -z "$K" ] && [ -f ~/.wj/profiles/adnx.json ] && K=$(jq -r .credential ~/.wj/profiles/adnx.json)
[ -z "$K" ] && { echo "Thieu key: export GATEWAY_KEY=..."; exit 1; }

URL=https://ai.adnx.vn/v1/messages
MODEL=cc/claude-haiku-4-5-20251001
H=(-H "Authorization: Bearer $K" -H 'anthropic-version: 2023-06-01'
   -H 'anthropic-beta: advanced-tool-use-2025-11-20' -H 'content-type: application/json'
   -H 'User-Agent: wj-gateway-client/1' -H 'x-app: wj')

# body REF DEFERRED_NAME STREAM(true|false|none)
body() {
  jq -nc --arg m "$MODEL" --arg r "$1" --arg n "$2" --arg s "$3" '
   {model:$m,max_tokens:300,
    tools:[{name:"ToolSearch",description:"search tools",input_schema:{type:"object",properties:{}}},
           {name:$n,description:"Watch. Takes city.",input_schema:{type:"object",properties:{city:{type:"string"}}},defer_loading:true}],
    messages:[{role:"user",content:"go"},
      {role:"assistant",content:[{type:"tool_use",id:"toolu_01",name:"ToolSearch",input:{}}]},
      {role:"user",content:[{type:"tool_result",tool_use_id:"toolu_01",content:[{type:"tool_reference",tool_name:$r}]},
                            {type:"text",text:"Call it with city Hanoi."}]}]}
   + (if $s=="none" then {} else {stream:($s=="true")} end)'
}

# call LABEL BODY
call() {
  out=$(curl -s -N --noproxy '*' -w '\n__HTTP__%{http_code}' "$URL" "${H[@]}" --data-binary "$2")
  code=${out##*__HTTP__}; out=${out%__HTTP__*}
  if printf '%s' "$out" | grep -q '^event:\|^data: {"type":"message_start"'; then
    js=$(printf '%s\n' "$out" | sed -n 's/^data: //p' | grep '^{')
    tu=$(printf '%s\n' "$js" | jq -r 'select(.type=="content_block_start" and .content_block.type=="tool_use")|.content_block.name' 2>/dev/null | paste -sd, -)
    txt=""
  else
    js=$(printf '%s' "$out" | sed 's/data: \[DONE\]$//')
    tu=$(printf '%s' "$js" | jq -r '[.content[]?|select(.type=="tool_use")|.name]|join(",")' 2>/dev/null | head -1)
    txt=$(printf '%s' "$js" | jq -r '[.content[]?|select(.type=="text")|.text]|join(" ")' 2>/dev/null | head -1 | cut -c1-300)
  fi
  err=$(printf '%s' "$out" | grep -o "Tool reference '[^']*' not found in available tools" | head -1)
  rid=$(printf '%s' "$out" | grep -o 'req_[A-Za-z0-9]*' | head -1)
  printf '%-34s | HTTP %s | tool_use=[%s] | %s | %s\n' "$1" "$code" "$tu" "${err:-}" "${rid:-}"
  [ -n "$txt" ] && printf '%-34s   model noi: %s\n' "" "$txt"
}

echo "Thoi diem: $(date -u +%Y-%m-%dT%H:%M:%SZ)  model: $MODEL"
B1=$(jq -nc --arg m "$MODEL" '{model:$m,max_tokens:100,stream:false,
  tools:[{name:"ToolSearch",description:"search tools",input_schema:{type:"object",properties:{}}},
         {name:"Monitor",description:"Watch.",input_schema:{type:"object",properties:{}},defer_loading:true}],
  messages:[{role:"user",content:"hi"}]}')
call "T1 baseline (khong tool_reference)" "$B1"
call "T2 ref=Monitor (ten goc)"           "$(body Monitor Monitor false)"
call "T3 ref=Monitor_ide (ten + _ide)"    "$(body Monitor_ide Monitor false)"
call "T4 ref=mcp__kitwright__... (goc)"   "$(body mcp__kitwright__get_scene_info mcp__kitwright__get_scene_info false)"
call "T5 ref=Monitor, stream=true"        "$(body Monitor Monitor true)"
call "T6 doi chung: ref=Nope (khong co)"  "$(body Nope Monitor false)"
call "T7 ref=Monitor_ide, KHONG co stream" "$(body Monitor_ide Monitor none)"
B8=$(jq -nc --arg m "$MODEL" '{model:$m,max_tokens:600,stream:false,
  tools:[{name:"get_weather",description:"Weather.",input_schema:{type:"object",properties:{}}}],
  messages:[{role:"user",content:"Do not call any tool. List the exact names of ALL tools available to you, comma separated, character for character, nothing else."}]}')
call "T8 model thay nhung tool nao"       "$B8"
