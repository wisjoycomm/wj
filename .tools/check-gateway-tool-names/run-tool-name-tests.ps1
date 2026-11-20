# Tai hien loi tool_reference cua mot gateway (proxy API ben thu ba). Windows PowerShell 5.1+ / pwsh.
# Gateway: dat $env:GATEWAY_URL + $env:GATEWAY_KEY (+ $env:GATEWAY_MODEL), hoac $env:WJ_PROFILE=<ten>
# de lay tu ~/.wj/profiles/<ten>.json.
# Goi THANG gateway, khong qua proxy (proxy wj se sua request va che mat loi).
# Chay: powershell -ExecutionPolicy Bypass -File run-tool-name-tests.ps1
Remove-Item Env:HTTPS_PROXY, Env:HTTP_PROXY -ErrorAction SilentlyContinue
$Url = $env:GATEWAY_URL; $K = $env:GATEWAY_KEY; $Model = $env:GATEWAY_MODEL
if ($env:WJ_PROFILE) {
  $wj = Join-Path $HOME ".wj\profiles\$($env:WJ_PROFILE).json"
  if (Test-Path $wj) {
    $p = Get-Content $wj -Raw | ConvertFrom-Json
    if (-not $Url) { $Url = $p.base_url }
    if (-not $K) { $K = $p.credential }
    if (-not $Model) { $Model = @($p.models | Where-Object { $_ -match 'haiku' })[0] }
  }
}
if (-not $Url -or -not $K) { Write-Host 'Thieu gateway: dat $env:GATEWAY_URL va $env:GATEWAY_KEY, hoac $env:WJ_PROFILE'; exit 1 }
if (-not $Model) { Write-Host 'Thieu model: dat $env:GATEWAY_MODEL (model re nhat cua gateway, vd ban haiku)'; exit 1 }
$Url = ($Url.TrimEnd('/') -replace '/v1$', '') + '/v1/messages'
$f = Join-Path $env:TEMP "gw-test-body.json"

function Send($label, $obj) {
  [IO.File]::WriteAllText($f, ($obj | ConvertTo-Json -Depth 20 -Compress))
  $out = curl.exe -s -N --noproxy "*" -w "`n__HTTP__%{http_code}" $Url `
    -H "Authorization: Bearer $K" -H "anthropic-version: 2023-06-01" `
    -H "anthropic-beta: advanced-tool-use-2025-11-20" -H "content-type: application/json" `
    -H "User-Agent: wj-gateway-client/1" -H "x-app: wj" --data-binary "@$f"
  $out = ($out -join "`n")
  $code = ($out -split "__HTTP__")[-1].Trim()
  $body = ($out -split "__HTTP__")[0]
  $err = [regex]::Match($body, "Tool reference '[^']*' not found in available tools").Value
  $rid = [regex]::Match($body, "req_[A-Za-z0-9]+").Value
  $names = [regex]::Matches($body, '"type"\s*:\s*"tool_use"[^}]*?"name"\s*:\s*"([^"]+)"') | ForEach-Object { $_.Groups[1].Value }
  "{0,-36} | HTTP {1} | tool_use=[{2}] | {3} {4}" -f $label, $code, ($names -join ","), $err, $rid
  $txt = [regex]::Matches($body, '"type"\s*:\s*"text"\s*,\s*"text"\s*:\s*"([^"]*)"') | ForEach-Object { $_.Groups[1].Value }
  if ($txt -and $code -eq "200") { "{0,-36}   model noi: {1}" -f "", ($txt -join " ") }
}

function Body($ref, $name = $ref, $stream = "false") {
  $o = [ordered]@{ model = $Model; max_tokens = 300
    tools = @(
      @{ name = "ToolSearch"; description = "search tools"; input_schema = @{ type = "object"; properties = @{} } },
      @{ name = $name; description = "Watch. Takes city."; input_schema = @{ type = "object"; properties = @{ city = @{ type = "string" } } }; defer_loading = $true })
    messages = @(
      @{ role = "user"; content = "go" },
      @{ role = "assistant"; content = @(@{ type = "tool_use"; id = "toolu_01"; name = "ToolSearch"; input = @{} }) },
      @{ role = "user"; content = @(
          @{ type = "tool_result"; tool_use_id = "toolu_01"; content = @(@{ type = "tool_reference"; tool_name = $ref }) },
          @{ type = "text"; text = "Call it with city Hanoi." }) }) }
  if ($stream -ne "none") { $o.stream = ($stream -eq "true") }
  $o
}

"Thoi diem: {0}  model: {1}" -f (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ"), $Model
Send "T1 baseline (khong tool_reference)" @{ model = $Model; max_tokens = 100; stream = $false
  tools = @(@{ name = "ToolSearch"; description = "search tools"; input_schema = @{ type = "object"; properties = @{} } })
  messages = @(@{ role = "user"; content = "hi" }) }
Send "T2 ref=Monitor (ten goc)"            (Body "Monitor")
Send "T3 ref=Monitor_ide (ten + _ide)"     (Body "Monitor_ide" "Monitor")
Send "T4 ref=mcp__kitwright__... (goc)"    (Body "mcp__kitwright__get_scene_info")
Send "T5 ref=Monitor, stream=true"         (Body "Monitor" "Monitor" "true")
Send "T6 doi chung: ref=Nope (khong co)"   (Body "Nope" "Monitor")
Send "T7 ref=Monitor_ide, KHONG co stream" (Body "Monitor_ide" "Monitor" "none")
Send "T8 model thay nhung tool nao" @{ model = $Model; max_tokens = 600; stream = $false
  tools = @(@{ name = "get_weather"; description = "Weather."; input_schema = @{ type = "object"; properties = @{} } })
  messages = @(@{ role = "user"; content = "Do not call any tool. List the exact names of ALL tools available to you, comma separated, character for character, nothing else." }) }
Remove-Item $f -ErrorAction SilentlyContinue
