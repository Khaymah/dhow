#!/usr/bin/env bash
# shellcheck disable=SC2034,SC2154

captures=
events_pid=
log_lines=50000

noisy_step() {
  local extra=$work/noisy-step
  cat >"$extra" <<EOF
          if [ -e noisy ]; then seq 1 $log_lines | sed 's/^/line /'; printf '\033[31mred\033[0m\n<img src=x onerror=alert(1)>\n\342\200\256bidi\n'; fi
EOF
  sed -i "/echo ok > site\/healthz; fi/r $extra" "$work/repo/.dhow.yml"
  grep -q 'if \[ -e noisy \]' "$work/repo/.dhow.yml" && readable_repo
}

eval "m6_write_pipeline() $(declare -f write_pipeline | tail -n +2)"
write_pipeline() { m6_write_pipeline && noisy_step; }

capture() {
  install -d -m 0700 "$captures"
  api "$work/capture" GET "$2" '' "${session[@]}" >/dev/null || return 1
  cp -- "$work/capture.body" "$captures/$1.json"
}

capture_all() {
  capture channels "$(project_path)/notification-channels" &&
    capture rules "$(project_path)/notification-rules" &&
    capture deliveries "$(project_path)/notification-deliveries?Limit=100" &&
    capture inbox /api/v1/me/notifications &&
    capture unread /api/v1/me/notifications/unread-count &&
    capture doctor /api/v1/system/doctor &&
    capture overview "/api/v1/organizations/$org_id/overview" &&
    capture board "$(project_path)/deploy-board"
}

doctor_is_plain() {
  grep -q '"ServerBuild"' "$captures/doctor.json" && ! grep -q '"Detail"' "$captures/doctor.json"
}

start_events() {
  install -d -m 0700 "$captures"
  : >"$captures/events.txt"
  printf 'header = "Cookie: %s"\nheader = "Origin: %s"\n' "$(curl_quote "$session_cookie")" "$public_url" |
    docker compose exec -T tools curl --config - --silent --no-buffer --max-time 3000 --header 'Accept: text/event-stream' \
      -- "$base/api/v1/organizations/$org_id/events" >"$captures/events.txt" 2>"$work/events.err" &
  events_pid=$!
  for _ in $(seq 30); do
    grep -q . "$captures/events.txt" 2>/dev/null && return 0
    sleep 1
  done
  [[ -z $(cat -- "$work/events.err") ]] && kill -0 "$events_pid" 2>/dev/null
}

events_saw() {
  for _ in $(seq 120); do
    grep -q "$1" "$captures/events.txt" && return 0
    sleep 1
  done
  why "the event stream never carried $1: $(tail -c 600 "$captures/events.txt")"
  return 1
}

events_refuse_token() {
  local status
  status=$(api "$work/events-token" GET "/api/v1/organizations/$org_id/events" '' --header "$(bearer)" --max-time 5) || true
  [[ $status == 401 || $status == 403 ]] || { why "an API token got $status from the event stream"; return 1; }
}

stop_events() {
  [[ -z $events_pid ]] || kill "$events_pid" 2>/dev/null || true
  events_pid=
}

failed_deploy() {
  ship_release 'release two' broken 'Release two, broken on purpose' || return 1
  run_failed_with "$current_run" DEPLOY_ROLLED_BACK || return 1
  failed_number=$(run_number "$current_run")
  [[ -n $failed_number ]]
}

good_deploy() {
  ship_release 'release three' healthy 'Release three' || return 1
  run_ends "$current_run" || return 1
  ok_number=$(run_number "$current_run")
  [[ -n $ok_number ]]
}

big_log() {
  write_site 'release four' healthy
  : >"$work/repo/noisy"
  readable_repo
  push_and_wait 'Print a big log' || return 1
  run_ends "$current_run" || return 1
  run_detail "$current_run" || return 1
  local job want got
  want=$(seq 1 "$log_lines" | sed 's/^/line /' | sha256sum | cut -d' ' -f1)
  for job in $(grep -o '"CurrentJobId":"[^"]*"' "$work/run.body" | cut -d'"' -f4); do
    [[ $(api "$work/raw" GET "/api/v1/organizations/$org_id/jobs/$job/logs/raw" '' "${session[@]}") == 200 ]] || continue
    got=$(grep -o 'line [0-9][0-9]*' "$work/raw.body" | sha256sum | cut -d' ' -f1)
    [[ $got == "$want" ]] || continue
    raw_job=$job
    return 0
  done
  why "no job log has the $log_lines lines in order; the last one has $(grep -c 'line [0-9]' "$work/raw.body") of them"
  return 1
}

raw_log_is_safe_text() {
  local head=$work/raw.head
  grep -qi '^content-type: text/plain' "$head" && grep -qi '^content-disposition: attachment' "$head" &&
    grep -qi '^x-content-type-options: nosniff' "$head" || { why "the headers of the raw log: $(tr -d '\r' <"$head" | tr '\n' '|')"; return 1; }
  grep -qF '<img src=x onerror=alert(1)>' "$work/raw.body" && grep -q $'\x1b\\[31mred' "$work/raw.body" && grep -q $'\xe2\x80\xaebidi' "$work/raw.body"
}

killed_while_sending() {
  local before
  before=$(attempts_of webhook)
  rcall POST '/_hold?service=webhook&ms=25000' >/dev/null || return 1
  test_send kill "$ch_hook" || return 1
  held_event=$test_event
  for _ in $(seq 60); do
    [[ $(attempts_of webhook) -gt $before ]] && break
    sleep 1
  done
  [[ $(attempts_of webhook) -gt $before ]] || { why "the server never sent the held message"; return 1; }
  docker compose kill -s KILL server >/dev/null 2>&1 || return 1
  docker compose start server >/dev/null 2>&1 || return 1
  wait_http "$base/api/v1/health/ready" 900 || { why "the server never answered after the kill"; return 1; }
  rcall POST '/_hold?service=webhook&ms=0' >/dev/null
}

sent_once_or_again_with_the_same_key() {
  local n
  ARRIVE_WAIT=60 arrives "key=$held_event" || return 1
  deliveries_settled 300 || return 1
  n=$(rcount "key=$held_event")
  ((n >= 1 && n <= 2)) || { why "the held message reached the receiver $n times"; return 1; }
  [[ $(rcount service=webhook signed=1 "key=$held_event") == "$n" ]]
}

dead_then_retried() {
  local id
  rcall POST '/_script?service=webhook&status=400' >/dev/null || return 1
  test_send dead "$ch_hook" || return 1
  dead_event=$test_event
  for _ in $(seq 90); do
    deliveries_in dead >/dev/null
    grep -qF "\"EventId\":\"$dead_event\"" "$work/dl-dead.body" && break
    sleep 1
  done
  grep -qF "\"EventId\":\"$dead_event\"" "$work/dl-dead.body" || { why "the delivery never went dead: $(head -c 500 "$work/dl-dead.body")"; return 1; }
  id=$(sed 's/},{/}\n{/g' "$work/dl-dead.body" | grep -F "\"EventId\":\"$dead_event\"" | grep -o '"Id":"[^"]*"' | head -1 | cut -d'"' -f4)
  grep -qF '"LastStatus":400' "$work/dl-dead.body" && grep -qF '"LastError":"http_status"' "$work/dl-dead.body" || return 1
  [[ $(rcount "key=$dead_event") == 0 ]] || return 1
  ok_status "$work/retry" "$(api "$work/retry" POST "$(project_path)/notification-deliveries/$id/retry" '' "${session[@]}")" 200 || return 1
  arrives service=webhook signed=1 "key=$dead_event"
}
