# Copyright (c) 2020 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

__gate_complete_module()
{
	local gate cur output

	gate="${COMP_WORDS[0]}"
	cur="${COMP_WORDS[$COMP_CWORD]}"

	if echo "$cur" | grep -q /; then
		COMPREPLY=( $( compgen -f -- "$cur" ) )
	else
		if [ -z "$2" ] || [ "$1" = push ]; then
			output=$( "$gate" modules 2>/dev/null )
		else
			output=$( "$gate" "$2" modules 2>/dev/null )
		fi
		COMPREPLY=( $( compgen -W "$output" -- "$cur" ) )
	fi
}

__gate_complete_instance()
{
	local gate cur output

	gate="${COMP_WORDS[0]}"
	cur="${COMP_WORDS[$COMP_CWORD]}"

	if [ -z "$2" ]; then
		output=$( "$gate" instances 2>/dev/null | cut -d" " -f1 )
	else
		output=$( "$gate" "$2" instances 2>/dev/null | cut -d" " -f1 )
	fi
	COMPREPLY=( $( compgen -W "$output" -- "$cur" ) )
}

__gate_completion()
{
	local addr cmd kind ignore i cur

	COMPREPLY=()

	addr=
	cmd=
	kind=address-command
	ignore=false

	for ((i=1; i < $COMP_CWORD; i++)); do
		cur="${COMP_WORDS[$i]}"

		if $ignore; then
			ignore=false
		elif [ "${cur::1}" = "-" ]; then
			ignore=true
		else
			case $kind in
				address-command)
					if echo "$cur" | grep -qE '(\.|://)'; then
						addr="$cur"
						kind=command
					else
						cmd="$cur"
						case "$cmd" in
							call|launch) kind=module ;;
							delete|io|kill|resume|status|suspend|wait) kind=instance ;;
							pull|push) kind=address2 ;;
							*) return ;;
						esac
					fi
					;;

				command)
					cmd="$cur"
					case "$cmd" in
						call|download|launch|unref) kind=module ;;
						delete|io|kill|repl|resume|snapshot|status|suspend|wait) kind=instance ;;
						*) return ;;
					esac
					;;

				address2)
					addr="$cur"
					kind=module
					;;

				*)
					return
					;;
			esac
		fi
	done

	if $ignore; then
		return
	fi

	cur="${COMP_WORDS[$COMP_CWORD]}"

	case $kind in
		address-command)
			COMPREPLY=( $( compgen -W "call delete io kill launch pull push resume status suspend wait" -- "$cur" ) )
			;;

		command)
			COMPREPLY=( $( compgen -W "call delete download io kill launch repl resume snapshot status suspend unref wait" -- "$cur" ) )
			;;

		module)
			__gate_complete_module $cmd "$addr"
			;;

		instance)
			__gate_complete_instance $cmd "$addr"
			;;

		*)
			return
			;;
	esac
}

complete -F __gate_completion gate
