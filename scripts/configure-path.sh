#!/usr/bin/env bash
# Sourced by the installer. Arguments keep profile writes independently testable.
aegis_configure_path() (
    local aegis_profile_root="$1"
    local aegis_binary_dir="$2"
    local aegis_login_shell="${3##*/}"
    local aegis_config_root="${4:-$aegis_profile_root/.config}"
    local aegis_quoted aegis_line aegis_profile
    local aegis_profiles=()
    umask 077
    case "$aegis_binary_dir" in
        *:*|*$'\n'*|*$'\r'*) echo 'The installation directory cannot contain colons or newlines.' >&2; return 1;;
    esac
    if [ "$aegis_login_shell" = fish ]; then
        aegis_quoted="${aegis_binary_dir//\\/\\\\}"
        aegis_quoted="'${aegis_quoted//\'/\\\'}'"
        aegis_line="contains -- $aegis_quoted \$PATH; or set -gx PATH $aegis_quoted \$PATH"
        aegis_profiles=("$aegis_config_root/fish/conf.d/aegis.fish")
    else
        # POSIX shell quoting keeps spaces, apostrophes and dollar signs literal.
        aegis_quoted="'${aegis_binary_dir//\'/\'\\\'\'}'"
        aegis_line="case \":\$PATH:\" in *:$aegis_quoted:*) ;; *) export PATH=$aegis_quoted:\"\$PATH\" ;; esac"
        case "$aegis_login_shell" in
            zsh) aegis_profiles=("$aegis_profile_root/.zshrc");;
            bash)
                aegis_profiles=("$aegis_profile_root/.bashrc")
                if [ -f "$aegis_profile_root/.bash_profile" ]; then
                    aegis_profiles+=("$aegis_profile_root/.bash_profile")
                elif [ -f "$aegis_profile_root/.bash_login" ]; then
                    aegis_profiles+=("$aegis_profile_root/.bash_login")
                else
                    aegis_profiles+=("$aegis_profile_root/.profile")
                fi
                ;;
            *) aegis_profiles=("$aegis_profile_root/.profile");;
        esac
    fi
    for aegis_profile in "${aegis_profiles[@]}"; do
        mkdir -p "$(dirname "$aegis_profile")"
        if [ ! -f "$aegis_profile" ] || ! grep -Fqx -- "$aegis_line" "$aegis_profile"; then
            printf '\n# Aegis executable directory\n%s\n' "$aegis_line" >> "$aegis_profile"
        fi
        printf 'PATH configured in: %s\n' "$aegis_profile"
    done
)
