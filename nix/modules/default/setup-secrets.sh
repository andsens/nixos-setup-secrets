#!/usr/bin/env bash
set -eo pipefail; shopt -s inherit_errexit
PATH=@PATH@
source "@records_sh@/records.sh"

main() {
  DOC="setup-secrets
Usage:
  setup-secrets [--auto|--show]

Options:
  --auto  Don't show edit dialog, fetch the secrets and immediately store them
  --show  Show the secret values
"
# docopt parser below, refresh this parser with `docopt.sh setup-secrets.sh`
# shellcheck disable=2016,2086,2329,1090,1091,2034
docopt() { local v='2.0.3'; source "@docopt_sh@/docopt-lib.sh" "$v" || { ret=$?
printf -- "exit %d\n" "$ret";exit "$ret";};set -e;trimmed_doc=${DOC:0:174}
usage=${DOC:14:38};digest=43ba0;options=(' --auto 0' ' --show 0');node_0(){
switch __auto 0;};node_1(){ switch __show 1;};node_2(){ choice 0 1;};node_3(){
optional 2;};cat <<<' docopt_exit() { [[ -n $1 ]] && printf "%s\n" "$1" >&2
printf "%s\n" "${DOC:14:38}" >&2;exit 1;}';local varnames=(__auto __show) \
varname;for varname in "${varnames[@]}"; do unset "var_$varname";done;parse 3 \
"$@";local p=${DOCOPT_PREFIX:-''};for varname in "${varnames[@]}"; do unset \
"$p$varname";done;eval $p'__auto=${var___auto:-false};'$p'__show=${var___show:'\
'-false};';local docopt_i=1;[[ $BASH_VERSION =~ ^4.3 ]] && docopt_i=2;for \
((;docopt_i>0;docopt_i--)); do for varname in "${varnames[@]}"; do declare -p \
"$p$varname";done;done;}
# docopt parser above, complete command for generating this parser is `docopt.sh --library='"@docopt_sh@/docopt-lib.sh"' setup-secrets.sh`
  eval "$(docopt "$@")"
  # shellcheck disable=SC2016
  local fetch=(
    @fetch@
  )
  local i name description cmd value old_vars=() namemap=() dialog_pid
  # shellcheck disable=SC2154
  if $__auto; then
    exec 3> >(cat)
  else
    exec 3> >(dialog --programbox "Fetching secrets" 25 80)
  fi
  dialog_pid=$!
  {
    for (( i = 0; i < ${#fetch[@]}; i=i+3 )); do
      name=${fetch[$i]}
      namemap+=("$name")
      description=${fetch[$((i+1))]}
      cmd=${fetch[$((i+2))]}
      info 'Fetching "%s"' "$description"
      value=$(eval "$cmd")
      old_vars+=("OLD_$name=$(printf '%s' "$value")")
      # label y x item y x flen ilen
      args+=("$description" $((1+i/3)) 0 "$value" $((1+i/3)) 40 40 256)
    done
  } >&3 2>&3
  exec 3<&-
  wait $dialog_pid
  if $__auto; then
    vars=("${old_vars[@]}")
  else
    local data form=form
    # shellcheck disable=SC2154
    $__show || form=passwordform
    # text height width formheight
    data=$(dialog --stdout --$form \
      "Setup Secrets" 25 80 10 \
      "${args[@]}"
    )
    local lines vars=()
    readarray -t -d$'\n' lines <<<"$data"
    for (( i = 0; i < ${#lines[@]}; i++ )); do
      name=${namemap[$i]}
      value=${lines[$i]}
      debug "%s=%s" "$name" "$value"
      vars+=("$name=$(printf '%s' "$value")")
    done
  fi

  # shellcheck disable=SC2016
  local store=(
    @store@
  )
  if $__auto; then
    exec 3> >(cat)
  else
    exec 3> >(dialog --programbox "Storing secrets" 25 80)
  fi
  dialog_pid=$!
  {
    for (( i = 0; i < ${#store[@]}; i=i+3 )); do
      log_entry=${store[$i]}
      cmd=${store[$((i+1))]}
      info '%s' "$log_entry"
      (
        local var
        for var in "${vars[@]}"; do eval "$var"; done
        for var in "${old_vars[@]}"; do eval "$var"; done
        eval "$cmd"
      )
    done
  } >&3 2>&3
  exec 3<&-
  wait $dialog_pid
}

main "$@"
