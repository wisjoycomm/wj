#!/usr/bin/env bash
# Ghi log HTTP day du cho tung test (request body, header gui di, status, header + body tra ve).
# Header Authorization duoc che. Can: bash, curl, jq.
# Chay: bash capture-logs.sh <thu-muc-ra>
unset HTTPS_PROXY HTTP_PROXY https_proxy http_proxy ALL_PROXY all_proxy
K="${GATEWAY_KEY:-}"
[ -z "$K" ] && [ -f ~/.wj/profiles/adnx.json ] && K=$(jq -r .credential ~/.wj/profiles/adnx.json)
[ -z "$K" ] && { echo "Thieu key: export GATEWAY_KEY=..."; exit 1; }
OUT="${1:-logs}"; mkdir -p "$OUT"

URL=https://ai.adnx.vn/v1/messages
MODEL=cc/claude-haiku-4-5-20251001
HDRS=('anthropic-version: 2023-06-01' 'anthropic-beta: advanced-tool-use-2025-11-20'
      'content-type: application/json' 'User-Agent: wj-gateway-client/1' 'x-app: wj')

body() { # REF DEFERRED_NAME STREAM(true|false|none)
  jq --arg m "$MODEL" --arg r "$1" --arg n "$2" --arg s "$3" -n '
   {model:$m,max_tokens:300,
    tools:[{name:"ToolSearch",description:"search tools",input_schema:{type:"object",properties:{}}},
           {name:$n,description:"Watch. Takes city.",input_schema:{type:"object",properties:{city:{type:"string"}}},defer_loading:true}],
    messages:[{role:"user",content:"go"},
      {role:"assistant",content:[{type:"tool_use",id:"toolu_01",name:"ToolSearch",input:{}}]},
      {role:"user",content:[{type:"tool_result",tool_use_id:"toolu_01",content:[{type:"tool_reference",tool_name:$r}]},
                            {type:"text",text:"Call it with city Hanoi."}]}]}
   + (if $s=="none" then {} else {stream:($s=="true")} end)'
}

capture() { # ID TITLE BODY
  f="$OUT/$1.log"; req="$OUT/.req.json"; printf '%s' "$3" > "$req"
  args=(); for h in "${HDRS[@]}"; do args+=(-H "$h"); done
  {
    echo "### $1 - $2"
    echo "### Thoi diem gui (UTC): $(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)"
    echo
    echo ">>> POST $URL"
    echo ">>> Authorization: Bearer <da che>"
    for h in "${HDRS[@]}"; do echo ">>> $h"; done
    echo ">>>"
    printf '%s' "$3" | jq .
    echo
    echo "<<< RESPONSE"
  } > "$f"
  curl -s -N --noproxy '*' -D "$OUT/.hdr" -o "$OUT/.body" \
       -w '%{http_code} %{time_total}s %{remote_ip}' \
       "$URL" -H "Authorization: Bearer $K" "${args[@]}" --data-binary "@$req" > "$OUT/.meta"
  {
    sed 's/\r$//; s/^/<<< /' "$OUT/.hdr"
    cat "$OUT/.body"; echo
    echo
    echo "### status / thoi gian / IP: $(cat "$OUT/.meta")"
  } >> "$f"
  rm -f "$OUT/.hdr" "$OUT/.body" "$OUT/.meta" "$req"
  echo "$1: $(head -c 3 <<<"$(grep -m1 '^<<< HTTP' "$f" | awk '{print $3}')") -> $f"
}

B1=$(jq -nc --arg m "$MODEL" '{model:$m,max_tokens:100,stream:false,
  tools:[{name:"ToolSearch",description:"search tools",input_schema:{type:"object",properties:{}}},
         {name:"Monitor",description:"Watch.",input_schema:{type:"object",properties:{}},defer_loading:true}],
  messages:[{role:"user",content:"hi"}]}')
capture T1 "baseline, khong co tool_reference (mong doi 200)" "$B1"
capture T2 "tool_reference = Monitor, ten goc (LOI CHINH: ra 400)" "$(body Monitor Monitor false)"
capture T3 "tool_reference = Monitor_ide (ra 200: chung minh gateway da doi ten)" "$(body Monitor_ide Monitor false)"
capture T4 "tool_reference = ten MCP goc (ra 400)" "$(body mcp__kitwright__get_scene_info mcp__kitwright__get_scene_info false)"
capture T5 "tool_reference = Monitor, stream=true (ra 400)" "$(body Monitor Monitor true)"
capture T6 "doi chung: tool_reference = Nope, khong ton tai (400 cung thong bao)" "$(body Nope Monitor false)"
capture T7 "tool_reference = Monitor_ide, request KHONG co truong stream (loi phu: lo ten _ide)" "$(body Monitor_ide Monitor none)"
capture T7b "giong T7 nhung co stream=true (doi chung: ten duoc doi lai)" "$(body Monitor_ide Monitor true)"
B8=$(jq -nc --arg m "$MODEL" '{model:$m,max_tokens:600,stream:false,
  tools:[{name:"get_weather",description:"Weather.",input_schema:{type:"object",properties:{}}}],
  messages:[{role:"user",content:"Do not call any tool. List the exact names of ALL tools available to you, comma separated, character for character, nothing else."}]}')
capture T8 "hoi model thay nhung tool nao (ta chi gui get_weather)" "$B8"
