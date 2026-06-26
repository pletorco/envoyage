package main

import (
	"flag"
	"fmt"
	"io"
)

func runCompletion(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("completion", flag.ContinueOnError)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("completion requires one shell: bash, zsh, fish, or powershell")
	}

	script, err := completionScript(flags.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, script)
	return nil
}

func completionScript(shell string) (string, error) {
	switch shell {
	case "bash":
		return bashCompletionScript, nil
	case "zsh":
		return zshCompletionScript, nil
	case "fish":
		return fishCompletionScript, nil
	case "powershell":
		return powershellCompletionScript, nil
	default:
		return "", fmt.Errorf("unsupported completion shell %q", shell)
	}
}

const bashCompletionScript = `# bash completion for envoyage
_envoyage()
{
    local cur prev sub commands flags
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    commands="compose completion decrypt encrypt env install keygen shim status uninstall version"

    case "$prev" in
        --env-file|--identity|--in|--out|-o|-i|--bin-dir|--lib-dir)
            COMPREPLY=( $(compgen -f -- "$cur") )
            return 0
            ;;
        --recipient|-r)
            return 0
            ;;
    esac

    if [[ $COMP_CWORD -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
        return 0
    fi

    sub="${COMP_WORDS[1]}"
    case "$sub" in
        compose)
            flags="--identity --env-file -f -p up down config exec logs ps pull build restart stop"
            ;;
        completion)
            flags="bash zsh fish powershell"
            ;;
        decrypt)
            flags="--in --out -o --identity -i --force"
            ;;
        encrypt)
            flags="--in --out -o --identity -i --recipient -r --force"
            ;;
        env)
            flags="extract inline --compose --write --env --secrets-env --secrets --out -o --env-file --identity -i --force"
            ;;
        install)
            flags="--system --bin-dir --lib-dir --force"
            ;;
        keygen)
            flags="--out -o --force"
            ;;
        shim)
            flags="status install uninstall --runtime auto docker podman all --system --bin-dir --force"
            ;;
        status|uninstall)
            flags="--system --bin-dir --lib-dir"
            ;;
        *)
            flags=""
            ;;
    esac
    COMPREPLY=( $(compgen -W "$flags" -- "$cur") )
}
complete -F _envoyage envoyage
`

const zshCompletionScript = `#compdef envoyage

_envoyage() {
  local -a commands
  commands=(
    'compose:run Docker Compose through Envoyage'
    'completion:generate shell completion script'
    'decrypt:decrypt .env.age to .secrets.env'
    'encrypt:encrypt .secrets.env to .env.age'
    'env:extract or inline Compose environment values'
    'install:install Envoyage'
    'keygen:generate an age identity'
    'shim:manage optional Docker/Podman shims'
    'status:show Envoyage install status'
    'uninstall:remove Envoyage install'
    'version:print Envoyage version'
  )

  _arguments -C \
    '1:command:->command' \
    '*::arg:->args'

  case "$state" in
    command)
      _describe -t commands 'envoyage command' commands
      ;;
    args)
      case "$words[2]" in
        compose)
          _arguments '*--identity[age identity path]:identity:_files' '*--env-file[dotenv or age env file]:env file:_files' '*::compose arg: _normal'
          ;;
        completion)
          _values 'shell' bash zsh fish powershell
          ;;
        decrypt)
          _arguments '--in[encrypted age input path]:input:_files' '--out[plaintext dotenv output path]:output:_files' '-o[plaintext dotenv output path]:output:_files' '--identity[age identity path]:identity:_files' '-i[age identity path]:identity:_files' '--force[overwrite output file]'
          ;;
        encrypt)
          _arguments '--in[plaintext dotenv input path]:input:_files' '--out[encrypted age output path]:output:_files' '-o[encrypted age output path]:output:_files' '--identity[age identity path]:identity:_files' '-i[age identity path]:identity:_files' '--recipient[age recipient]:recipient:' '-r[age recipient]:recipient:' '--force[overwrite output file]'
          ;;
        env)
          _arguments '1:env command:(extract inline)' '--compose[compose file path]:compose:_files' '--write[write env files and update compose]' '--env[non-secret dotenv output path]:env file:_files' '--secrets-env[secret dotenv output path]:secrets file:_files' '--secrets[split secret-looking keys]' '--out[rendered compose output path]:output:_files' '-o[rendered compose output path]:output:_files' '--env-file[dotenv or age env file]:env file:_files' '--identity[age identity path]:identity:_files' '-i[age identity path]:identity:_files' '--force[overwrite output file]'
          ;;
        install)
          _arguments '--system[use system-wide /usr/local paths]' '--bin-dir[command symlink directory]:directory:_files -/' '--lib-dir[binary install directory]:directory:_files -/' '--force[overwrite Envoyage install]'
          ;;
        keygen)
          _arguments '--out[age identity output path]:output:_files' '-o[age identity output path]:output:_files' '--force[overwrite identity file]'
          ;;
        shim)
          _arguments '1:shim command:(status install uninstall)' '--runtime[runtime shim to manage]:runtime:(auto docker podman all)' '--system[use system-wide /usr/local/bin path]' '--bin-dir[runtime shim symlink directory]:directory:_files -/' '--force[recreate shim]'
          ;;
        status|uninstall)
          _arguments '--system[use system-wide /usr/local paths]' '--bin-dir[command symlink directory]:directory:_files -/' '--lib-dir[binary install directory]:directory:_files -/'
          ;;
      esac
      ;;
  esac
}

_envoyage "$@"
`

const fishCompletionScript = `# fish completion for envoyage
complete -c envoyage -f -n "__fish_use_subcommand" -a "compose" -d "Run Docker Compose through Envoyage"
complete -c envoyage -f -n "__fish_use_subcommand" -a "completion" -d "Generate shell completion script"
complete -c envoyage -f -n "__fish_use_subcommand" -a "decrypt" -d "Decrypt .env.age to .secrets.env"
complete -c envoyage -f -n "__fish_use_subcommand" -a "encrypt" -d "Encrypt .secrets.env to .env.age"
complete -c envoyage -f -n "__fish_use_subcommand" -a "env" -d "Extract or inline Compose environment values"
complete -c envoyage -f -n "__fish_use_subcommand" -a "install" -d "Install Envoyage"
complete -c envoyage -f -n "__fish_use_subcommand" -a "keygen" -d "Generate an age identity"
complete -c envoyage -f -n "__fish_use_subcommand" -a "shim" -d "Manage optional Docker/Podman shims"
complete -c envoyage -f -n "__fish_use_subcommand" -a "status" -d "Show Envoyage install status"
complete -c envoyage -f -n "__fish_use_subcommand" -a "uninstall" -d "Remove Envoyage install"
complete -c envoyage -f -n "__fish_use_subcommand" -a "version" -d "Print Envoyage version"

complete -c envoyage -n "__fish_seen_subcommand_from completion" -f -a "bash zsh fish powershell"
complete -c envoyage -n "__fish_seen_subcommand_from compose" -l identity -r -F -d "Age identity path"
complete -c envoyage -n "__fish_seen_subcommand_from compose" -l env-file -r -F -d "Dotenv or age env file"
complete -c envoyage -n "__fish_seen_subcommand_from decrypt" -l in -r -F -d "Encrypted age input path"
complete -c envoyage -n "__fish_seen_subcommand_from decrypt" -l out -s o -r -F -d "Plaintext dotenv output path"
complete -c envoyage -n "__fish_seen_subcommand_from decrypt" -l identity -s i -r -F -d "Age identity path"
complete -c envoyage -n "__fish_seen_subcommand_from decrypt" -l force -d "Overwrite output file"
complete -c envoyage -n "__fish_seen_subcommand_from encrypt" -l in -r -F -d "Plaintext dotenv input path"
complete -c envoyage -n "__fish_seen_subcommand_from encrypt" -l out -s o -r -F -d "Encrypted age output path"
complete -c envoyage -n "__fish_seen_subcommand_from encrypt" -l identity -s i -r -F -d "Age identity path"
complete -c envoyage -n "__fish_seen_subcommand_from encrypt" -l recipient -s r -r -d "Age recipient"
complete -c envoyage -n "__fish_seen_subcommand_from encrypt" -l force -d "Overwrite output file"
complete -c envoyage -n "__fish_seen_subcommand_from env" -f -a "extract inline"
complete -c envoyage -n "__fish_seen_subcommand_from env" -l compose -r -F -d "Compose file path"
complete -c envoyage -n "__fish_seen_subcommand_from env" -l write -d "Write env files and update compose"
complete -c envoyage -n "__fish_seen_subcommand_from env" -l env -r -F -d "Non-secret dotenv output path"
complete -c envoyage -n "__fish_seen_subcommand_from env" -l secrets-env -r -F -d "Secret dotenv output path"
complete -c envoyage -n "__fish_seen_subcommand_from env" -l secrets -d "Split secret-looking keys"
complete -c envoyage -n "__fish_seen_subcommand_from env" -l out -s o -r -F -d "Rendered compose output path"
complete -c envoyage -n "__fish_seen_subcommand_from env" -l env-file -r -F -d "Dotenv or age env file"
complete -c envoyage -n "__fish_seen_subcommand_from env" -l identity -s i -r -F -d "Age identity path"
complete -c envoyage -n "__fish_seen_subcommand_from env" -l force -d "Overwrite output file"
complete -c envoyage -n "__fish_seen_subcommand_from install status uninstall" -l bin-dir -r -F -d "Command symlink directory"
complete -c envoyage -n "__fish_seen_subcommand_from install status uninstall" -l lib-dir -r -F -d "Binary install directory"
complete -c envoyage -n "__fish_seen_subcommand_from install status uninstall" -l system -d "Use system-wide /usr/local paths"
complete -c envoyage -n "__fish_seen_subcommand_from install" -l force -d "Overwrite Envoyage install"
complete -c envoyage -n "__fish_seen_subcommand_from keygen" -l out -s o -r -F -d "Age identity output path"
complete -c envoyage -n "__fish_seen_subcommand_from keygen" -l force -d "Overwrite identity file"
complete -c envoyage -n "__fish_seen_subcommand_from shim" -f -a "status install uninstall"
complete -c envoyage -n "__fish_seen_subcommand_from shim" -l bin-dir -r -F -d "Runtime shim symlink directory"
complete -c envoyage -n "__fish_seen_subcommand_from shim" -l runtime -x -a "auto docker podman all" -d "Runtime shim to manage"
complete -c envoyage -n "__fish_seen_subcommand_from shim" -l system -d "Use system-wide /usr/local/bin path"
complete -c envoyage -n "__fish_seen_subcommand_from shim" -l force -d "Recreate shim"
`

const powershellCompletionScript = `# PowerShell completion for envoyage
Register-ArgumentCompleter -Native -CommandName envoyage -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $commands = @('compose','completion','decrypt','encrypt','env','install','keygen','shim','status','uninstall','version')
    $tokens = $commandAst.CommandElements | ForEach-Object { $_.ToString() }

    if ($tokens.Count -le 2) {
        $commands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    $sub = $tokens[1]
    $values = switch ($sub) {
        'compose' { @('--identity','--env-file','-f','-p','up','down','config','exec','logs','ps','pull','build','restart','stop') }
        'completion' { @('bash','zsh','fish','powershell') }
        'decrypt' { @('--in','--out','-o','--identity','-i','--force') }
        'encrypt' { @('--in','--out','-o','--identity','-i','--recipient','-r','--force') }
        'env' { @('extract','inline','--compose','--write','--env','--secrets-env','--secrets','--out','-o','--env-file','--identity','-i','--force') }
        'install' { @('--system','--bin-dir','--lib-dir','--force') }
        'keygen' { @('--out','-o','--force') }
        'shim' { @('status','install','uninstall','--runtime','auto','docker','podman','all','--system','--bin-dir','--force') }
        { $_ -in @('status','uninstall') } { @('--system','--bin-dir','--lib-dir') }
        default { @() }
    }

    $values | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`
