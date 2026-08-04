package settings

import (
	"encoding/json"
	"fmt"
	"os"
)

// UninstallConfig mirrors InstallConfig; only the fields Uninstall needs.
type UninstallConfig struct {
	SettingsPath   string
	ChainStatePath string
}

type UninstallAction string

const (
	UninstallRestoredChain UninstallAction = "restored-chained-statusline"
	UninstallRemoved       UninstallAction = "removed-statusline"
	UninstallNotOurs       UninstallAction = "left-unchanged" // statusLine no longer points at IA METER
	UninstallNoop          UninstallAction = "noop"           // already uninstalled
)

type UninstallResult struct {
	Action UninstallAction
}

// Uninstall restores Claude Code's settings.json to its pre-IAMETER state:
// if a third-party statusLine was chained, it's restored; otherwise the
// statusLine key is removed entirely (only if it still is unmistakably
// IA METER's — a statusLine the user has since repointed elsewhere is left
// alone). Safe to call repeatedly (idempotent).
func Uninstall(cfg UninstallConfig) (UninstallResult, error) {
	doc, err := load(cfg.SettingsPath)
	if err != nil {
		return UninstallResult{}, err
	}

	existing, hasExisting, err := readStatusLine(doc)
	if err != nil {
		return UninstallResult{}, err
	}

	if !hasExisting || !isIAMeterCommand(existing.Command) {
		// Nothing to restore, or the user pointed statusLine somewhere
		// else since installing — don't clobber their choice.
		if err := removeChainState(cfg.ChainStatePath); err != nil {
			return UninstallResult{}, err
		}
		if !hasExisting {
			return UninstallResult{Action: UninstallNoop}, nil
		}
		return UninstallResult{Action: UninstallNotOurs}, nil
	}

	chained, err := LoadChainState(cfg.ChainStatePath)
	if err != nil {
		return UninstallResult{}, err
	}

	result := UninstallResult{}
	if chained != nil {
		entryBytes, err := json.Marshal(chained)
		if err != nil {
			return UninstallResult{}, fmt.Errorf("settings: marshal restored statusLine: %w", err)
		}
		doc[statusLineKey] = entryBytes
		result.Action = UninstallRestoredChain
	} else {
		delete(doc, statusLineKey)
		result.Action = UninstallRemoved
	}

	if err := save(cfg.SettingsPath, doc); err != nil {
		return UninstallResult{}, err
	}
	if err := removeChainState(cfg.ChainStatePath); err != nil {
		return UninstallResult{}, err
	}
	return result, nil
}

func removeChainState(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("settings: remove chain state: %w", err)
	}
	return nil
}
