package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/config"
	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/model"
)

type Rejected struct {
	ChangeID  string `json:"change_id"`
	Reason    string `json:"reason"`
	CreatedAt int64  `json:"created_at"`
}

type State struct {
	Initialized bool                          `json:"initialized"`
	Cursor      int64                         `json:"cursor"`
	Lexicon     map[string]model.LexiconEntry `json:"lexicon"`
	Pending     []model.Change                `json:"pending"`
	Rejected    []Rejected                    `json:"rejected"`
	LastSyncAt  int64                         `json:"last_sync_at"`
	LastError   string                        `json:"last_error"`
}

func Empty() State {
	return State{Lexicon: make(map[string]model.LexiconEntry), Pending: make([]model.Change, 0), Rejected: make([]Rejected, 0)}
}

func Load() (State, error) {
	_, statePath, _, err := config.Paths()
	if err != nil {
		return State{}, err
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Empty(), nil
		}
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("解析本地状态: %w", err)
	}
	if st.Lexicon == nil {
		st.Lexicon = make(map[string]model.LexiconEntry)
	}
	if st.Pending == nil {
		st.Pending = make([]model.Change, 0)
	}
	if st.Rejected == nil {
		st.Rejected = make([]Rejected, 0)
	}
	return st, nil
}

func Save(st State) error {
	_, statePath, _, err := config.Paths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := statePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, statePath)
}

type Lock struct{ file *os.File }

func AcquireLock() (*Lock, error) {
	_, _, lockPath, err := config.Paths()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("已有同步任务正在运行")
	}
	return &Lock{file: f}, nil
}

func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	return l.file.Close()
}

func MarkError(st *State, err error) {
	st.LastSyncAt = time.Now().UnixMilli()
	if err != nil {
		st.LastError = err.Error()
	} else {
		st.LastError = ""
	}
}
