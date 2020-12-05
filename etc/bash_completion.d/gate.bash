# Copyright (c) 2020 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

__gate_complete_module()
{
	local gate cur output

	gate="${COMP_WORDS[0]}"
	cur="${COMP_WORDS[$COMP_CWORD]}"

	if echo "$cur" | grep -q '[./]'; then
		COMPREPLY=( $( compgen -f -o plusdirs -- "$cur" ) )
	else
		if [ -z "$2" ] || [ "$1" = push ]; then
			output=$( "$gate" modules 2>/dev/null | cut -d" " -f1 )
		else
			output=$( "$gate" "$2" modules 2>/dev/null | cut -d" " -f1 )
		fi
		COMPREPLY=( $( compgen -f -o plusdirs -W "$output" -- "$cur" ) )
	fi

	i=0
	for x in ${COMPREPLY[*]}; do
		if [ -d "$x" ]; then
			x="$x/"
		else
			x="$x "
		fi
		COMPREPLY[i++]="$x"
	done
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
	optargs=true
	ignore=false

	for ((i=1; i < $COMP_CWORD; i++)); do
		cur="${COMP_WORDS[$i]}"

		if $ignore; then
			ignore=false
		elif [ "${cur::1}" = "-" ]; then
			ignore=$optargs
		else
			optargs=false
			case $kind in
				address-command)
					if echo "$cur" | grep -qE '(\.|://)'; then
						addr="$cur"
						kind=command
					else
						cmd="$cur"
						case "$cmd" in
							import) kind=filename ;;
							export) kind=module-filename ;;
							call|launch|pin|show|unpin) kind=module ;;
							debug|delete|io|kill|repl|resume|snapshot|status|suspend|update|wait) kind=instance ;;
							pull|push) kind=address2 ;;
							*) return ;;
						esac
					fi
					;;

				command)
					cmd="$cur"
					case "$cmd" in
						import) kind=filename ;;
						export) kind=module-filename ;;
						call|launch|pin|show|unpin) kind=module ;;
						debug|delete|io|kill|repl|resume|snapshot|status|suspend|update|wait) kind=instance ;;
						*) return ;;
					esac
					;;

				address2)
					addr="$cur"
					kind=module
					;;

				module-filename)
					kind=filename
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
			COMPREPLY=( $( compgen -W "call debug delete export import instances io kill launch modules pin pull push repl resume show snapshot status suspend unpin update wait" -- "$cur" ) )
			;;

		command)
			COMPREPLY=( $( compgen -W "call debug delete export import instances io kill launch modules pin repl resume show snapshot status suspend unpin update wait" -- "$cur" ) )
			;;

		filename)
			COMPREPLY=( $( compgen -f -- "$cur" ) )
			;;

		module|module-filename)
			__gate_complete_module $cmd "$addr"
			;;

		instance)
			__gate_complete_instance $cmd "$addr"
			;;

		*)
			return
			;;
	esac

	i=0
	for x in ${COMPREPLY[*]}; do
		if echo "$x" | grep -q '[^ /]$'; then
			x="$x "
		fi
		COMPREPLY[i++]="$x"
	done
}

complete -o nospace -F __gate_completion gate
