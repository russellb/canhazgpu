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
        # After '--' — delegate to the next command's completion function
        local cmd="${words[$((passthrough_index + 1))]}"
        if command -v "$cmd" >/dev/null; then
            # Set up COMP_WORDS and COMP_CWORD for the delegated command
            local subcmd_words=("${words[@]:$((passthrough_index + 1))}")
            local subcmd_cword=$((COMP_CWORD - passthrough_index - 1))
            COMP_WORDS=("${subcmd_words[@]}")
            COMP_CWORD=$subcmd_cword
            # Load the appropriate completion function
            local completion_loader
            completion_loader=$(complete -p "$cmd" 2>/dev/null | sed -n "s/.*-F \([a-zA-Z0-9_]*\).*/\1/p")
            if [[ -n "$completion_loader" && $(type -t "$completion_loader") == "function" ]]; then
                "$completion_loader"
                return
            fi
        fi
        # Fallback to filename completion
        compopt -o nospace
        COMPREPLY=( $(compgen -f -- "$cur") )
        return
    fi

    # Before '--', provide completion for canhazgpu itself
    local opts="admin run status --gpus --help"
    COMPREPLY=( $(compgen -W "$opts" -- "$cur") )
}
complete -F _canhazgpu_complete -o default -o bashdefault canhazgpu
