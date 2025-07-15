_canhazgpu_complete() {
    local cur prev words cword
    _init_completion || return

    # Find position of '--'
    local passthrough_index=-1
    for i in "${!words[@]}"; do
        if [[ "${words[$i]}" == "--" ]]; then
            passthrough_index=$i
            break
        fi
    done

    if [[ $passthrough_index -ge 0 && $COMP_CWORD -gt $passthrough_index ]]; then
        # After '--' — delegate to system's existing completion
        
        # If we're completing the command name itself (first word after --)
        if [[ $COMP_CWORD -eq $((passthrough_index + 1)) ]]; then
            # Complete command names from PATH
            COMPREPLY=( $(compgen -c -- "$cur") )
            return
        fi
        
        # For command arguments, try to use the system's existing completion
        local cmd="${words[$((passthrough_index + 1))]}"
        
        # Create a new completion context for the command after --
        local cmd_words=("${words[@]:$((passthrough_index + 1))}")
        local cmd_cword=$((COMP_CWORD - passthrough_index - 1))
        
        # Save current completion state
        local orig_words=("${COMP_WORDS[@]}")
        local orig_cword=$COMP_CWORD
        local orig_line="$COMP_LINE"
        local orig_point=$COMP_POINT
        
        # Set up completion environment for the target command
        COMP_WORDS=("${cmd_words[@]}")
        COMP_CWORD=$cmd_cword
        COMP_LINE="${cmd_words[*]}"
        COMP_POINT=${#COMP_LINE}
        
        # Try to trigger completion for the command
        if declare -F _completion_loader >/dev/null 2>&1; then
            _completion_loader "$cmd" 2>/dev/null
        fi
        
        # Get the completion specification for this command
        local comp_spec
        comp_spec=$(complete -p "$cmd" 2>/dev/null)
        
        if [[ -n "$comp_spec" ]]; then
            # Extract and call the completion function
            if [[ "$comp_spec" =~ -F[[:space:]]+([^[:space:]]+) ]]; then
                local comp_func="${BASH_REMATCH[1]}"
                if declare -F "$comp_func" >/dev/null 2>&1; then
                    "$comp_func" "$cmd" "$cur" "$prev" 2>/dev/null
                fi
            fi
        fi
        
        # If no completions were generated, fall back to file completion
        if [[ ${#COMPREPLY[@]} -eq 0 ]]; then
            COMPREPLY=( $(compgen -f -- "$cur") )
        fi
        
        # Restore original completion state
        COMP_WORDS=("${orig_words[@]}")
        COMP_CWORD=$orig_cword
        COMP_LINE="$orig_line"
        COMP_POINT=$orig_point
        
        return
    fi

    # Before '--', provide completion for canhazgpu itself
    case "$prev" in
        canhazgpu|chg)
            COMPREPLY=( $(compgen -W "admin reserve release run status report web help --help --redis-host --redis-port --redis-db" -- "$cur") )
            ;;
        admin)
            COMPREPLY=( $(compgen -W "--gpus --force --help" -- "$cur") )
            ;;
        reserve)
            COMPREPLY=( $(compgen -W "--gpus --gpu-ids -g -G --duration -d --help" -- "$cur") )
            ;;
        release)
            COMPREPLY=( $(compgen -W "--gpu-ids -G --help" -- "$cur") )
            ;;
        run)
            COMPREPLY=( $(compgen -W "--gpus --gpu-ids -g -G --timeout -t --help --" -- "$cur") )
            ;;
        status)
            COMPREPLY=( $(compgen -W "--json -j --help" -- "$cur") )
            ;;
        report)
            COMPREPLY=( $(compgen -W "--days --help" -- "$cur") )
            ;;
        web)
            COMPREPLY=( $(compgen -W "--port --host --help" -- "$cur") )
            ;;
        --duration)
            COMPREPLY=( $(compgen -W "30m 1h 2h 4h 8h 1d 2d" -- "$cur") )
            ;;
        --days)
            COMPREPLY=( $(compgen -W "1 3 7 14 30 60 90" -- "$cur") )
            ;;
        *)
            COMPREPLY=( $(compgen -W "admin reserve release run status report web help --help --redis-host --redis-port --redis-db" -- "$cur") )
            ;;
    esac
}
complete -F _canhazgpu_complete -o default -o bashdefault canhazgpu
complete -F _canhazgpu_complete -o default -o bashdefault chg
