#!/usr/bin/env bash
# shellcheck disable=SC2034,SC2154

notify_host=webhook.notify.test
project_title='Demo app <!channel> @everyone'
telegram_chat=-1001234567890
slack_url=
discord_url=
telegram_token=
hook_secret_value=
ch_hook=
ch_slack=
ch_discord=
ch_telegram=
ch_inbox=
test_event=
channel_id=

project() {
  [[ $(api "$work/project" POST "/api/v1/organizations/$org_id/projects" \
    "$(printf '{"Name":"%s","Slug":"demo-app"}' "$project_title")" --header "$(bearer)") == 2?? ]] || return 1
  project_id=$(json_field "$work/project.body" Id)
  [[ -n $project_id ]]
}

rand_hex() { od -An -tx1 -N"$1" /dev/urandom | tr -d ' \n'; }

make_values() {
  slack_url="https://hooks.slack.com/services/T$(rand_hex 5 | tr a-f A-F)/B$(rand_hex 5 | tr a-f A-F)/$(rand_hex 12)"
  discord_url="https://discord.com/api/webhooks/$(printf '%018d' $((RANDOM * RANDOM * RANDOM)))/$(rand_hex 32)"
  telegram_token="$(printf '%09d' $((RANDOM * RANDOM % 1000000000))):$(rand_hex 18 | cut -c1-35)"
  needle "slack webhook url" "$slack_url"
  needle "discord webhook url" "$discord_url"
  needle "telegram bot token" "$telegram_token"
}

rcall() {
  local method=$1 path=$2
  shift 2
  docker compose exec -T tools curl --silent --show-error --fail-with-body --max-time 30 --cacert "$work/notify/ca.pem" \
    --request "$method" "$@" -- "https://$notify_host$path" </dev/null
}

rcount() {
  local args=() kv
  for kv in "$@"; do args+=(--data-urlencode "$kv"); done
  rcall GET /_count -G "${args[@]}" | tr -d '\r\n'
}

attempts_of() { rcall GET /_stats | grep -o "\"$1\":[0-9]*" | head -1 | cut -d: -f2; }

notify_receiver() {
  install -d -m 0755 "$work/notify"
  docker compose up --detach tools >/dev/null || return 1
  docker compose exec -T -e CGO_ENABLED=0 tools go build -trimpath -o "$work/notify/notify-receiver" ./deploy/e2e-targets/notify-receiver </dev/null || {
    why "the notify receiver did not build"
    return 1
  }
  docker compose up --detach notify >/dev/null || return 1
  for _ in $(seq 60); do
    [[ -s $work/notify/ca.pem ]] && rcall GET /_health >/dev/null 2>&1 && return 0
    sleep 1
  done
  why "the notify receiver never answered over TLS with its own CA"
  return 1
}

create_channel() {
  local name=$1 kind=$2 locale=$3 extra=$4
  ok_status "$work/chan-$name" "$(api "$work/chan-$name" POST "$(project_path)/notification-channels" \
    "$(printf '{"Kind":"%s","Name":"%s","Locale":"%s",%s"CurrentFactor":{"Method":"password","Password":"%s"}}' "$kind" "$name" "$locale" "$extra" "$password")" \
    "${session[@]}")" 201 || return 1
  channel_id=$(json_field "$work/chan-$name.body" Id)
  [[ -n $channel_id ]]
}

channels() {
  create_channel webhook webhook en "\"Url\":\"https://$notify_host/hooks/demo\"," || return 1
  ch_hook=$channel_id
  hook_secret_value=$(json_field "$work/chan-webhook.body" Secret)
  [[ $hook_secret_value =~ ^dhow_whsec_[A-Za-z0-9_-]{43}$ ]] || { why "the answer carries no signing secret"; return 1; }
  needle "webhook signing secret" "$hook_secret_value"
  (umask 077 && printf '%s' "$hook_secret_value" >"$work/hook-secret")
  rcall POST /_secret --data-binary "@$work/hook-secret" >/dev/null || return 1
  create_channel slack slack ar "\"Url\":\"$slack_url\"," || return 1
  ch_slack=$channel_id
  create_channel discord discord en "\"Url\":\"$discord_url\"," || return 1
  ch_discord=$channel_id
  create_channel telegram telegram ar "\"BotToken\":\"$telegram_token\",\"ChatId\":\"$telegram_chat\"," || return 1
  ch_telegram=$channel_id
  create_channel inbox inapp en '' || return 1
  ch_inbox=$channel_id
}

secrets_shown_once() {
  local name value
  for name in webhook:"$hook_secret_value" slack:"$slack_url" discord:"$discord_url" telegram:"$telegram_token"; do
    value=${name#*:}
    api "$work/chan-get" GET "$(project_path)/notification-channels" '' "${session[@]}" >/dev/null || return 1
    if grep -qF -- "$value" "$work/chan-get.body"; then
      why "the list of channels shows the secret of ${name%%:*}"
      return 1
    fi
  done
  grep -q '"HasSecret":true' "$work/chan-get.body"
}

addresses_refused() {
  local url status
  for url in https://169.254.169.254/latest/meta-data https://127.0.0.1/hook https://10.231.130.10/hook http://webhook.notify.test/hooks/demo; do
    status=$(api "$work/ssrf" POST "$(project_path)/notification-channels" \
      "$(printf '{"Kind":"webhook","Name":"bad","Url":"%s","CurrentFactor":{"Method":"password","Password":"%s"}}' "$url" "$password")" "${session[@]}") || true
    [[ $status == 422 ]] || { why "$url answered $status and not 422"; return 1; }
  done
  status=$(api "$work/ssrf" POST "$(project_path)/notification-channels" \
    "$(printf '{"Kind":"slack","Name":"bad","Url":"https://evil.example.com/services/T0AAAAAAA/B0AAAAAAA/xxxxxxxxxxxxxxxxxxxxxxxx","CurrentFactor":{"Method":"password","Password":"%s"}}' "$password")" "${session[@]}") || true
  [[ $status == 422 ]] && grep -qF '"Code":"CHANNEL_HOST_NOT_ALLOWED"' "$work/ssrf.body" || { why "a Slack channel on another host answered $status"; return 1; }
  api "$work/chan-count" GET "$(project_path)/notification-channels" '' "${session[@]}" >/dev/null
  [[ $(grep -o '"HasSecret"' "$work/chan-count.body" | wc -l) == 5 ]]
}

test_send() {
  ok_status "$work/test-$1" "$(api "$work/test-$1" POST "$(project_path)/notification-channels/$2/test" '' "${session[@]}")" 202 || return 1
  test_event=$(json_field "$work/test-$1.body" EventId)
  [[ -n $test_event ]]
}

arrives() {
  local seconds=${ARRIVE_WAIT:-150} got
  for _ in $(seq "$seconds"); do
    got=$(rcount "$@" 2>/dev/null) || got=0
    ((got >= 1)) && return 0
    sleep 1
  done
  why "no message for: $*"
  return 1
}

never() {
  local seconds=$1 got
  shift
  sleep "$seconds"
  got=$(rcount "$@") || return 1
  [[ $got == 0 ]] || { why "$got messages for: $*"; return 1; }
}

test_messages_arrive() {
  test_send chat-slack "$ch_slack" && arrives service=slack 'contains=رسالة اختبار من داو' || return 1
  test_send chat-discord "$ch_discord" && arrives service=discord 'contains=Test message from Dhow' mentions=none || return 1
  test_send chat-telegram "$ch_telegram" && arrives service=telegram 'contains=رسالة اختبار من داو' || return 1
  test_send hook "$ch_hook" && arrives service=webhook signed=1 "key=$test_event" || return 1
}

failed_run_notified() {
  local n=$1
  arrives service=slack 'contains=المشروع' "contains=رقم التنفيذ: $n" 'absent=Project:' || return 1
  arrives service=discord 'contains=Project:' "contains=Run number: $n" 'absent=المشروع' || return 1
  arrives service=telegram 'contains=المشروع' "contains=رقم التنفيذ: $n" 'absent=Project:' || return 1
  arrives service=webhook signed=1 "contains=\"Run\":$n" || return 1
}

mentions_neutralised() {
  arrives service=slack 'contains=&lt;!channel&gt;' 'contains=@everyone' || return 1
  arrives service=discord mentions=none 'contains=@everyone' || return 1
  arrives service=telegram 'contains=&lt;!channel&gt;' || return 1
  never 1 'contains=<!channel>'
}

not_notified() {
  local n=$1
  never 15 "contains=Run number: $n" && never 1 "contains=رقم التنفيذ: $n" && never 1 service=webhook "contains=\"Run\":$n"
}

deliveries_in() {
  api "$work/dl-$1" GET "$(project_path)/notification-deliveries?State=$1&Limit=100" '' "${session[@]}" >/dev/null || return 1
  grep -o '"ChannelId"' "$work/dl-$1.body" | wc -l
}

deliveries_settled() {
  local seconds=${1:-120} pending sending
  for _ in $(seq "$seconds"); do
    pending=$(deliveries_in pending) && sending=$(deliveries_in sending) && ((pending + sending == 0)) && return 0
    sleep 1
  done
  why "$pending deliveries still pending and $sending still sending"
  return 1
}

no_dead_deliveries() {
  [[ $(deliveries_in dead) == 0 ]] || { why "a delivery is dead: $(head -c 600 "$work/dl-dead.body")"; return 1; }
}

inbox_filled() {
  for _ in $(seq 60); do
    api "$work/inbox" GET /api/v1/me/notifications '' "${session[@]}" >/dev/null
    grep -q '"TitleKey":"inbox\.' "$work/inbox.body" && grep -q '"Read":false' "$work/inbox.body" && return 0
    sleep 1
  done
  why "the inbox holds: $(head -c 600 "$work/inbox.body")"
  return 1
}
