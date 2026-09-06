// Copyright 2025.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ui

import (
	"github.com/btafoya/lazysshterm/internal/core/domain"
	"github.com/rivo/tview"
)

// showSecretPrompt asks for a masked value (key passphrase or password) via
// a small form, then calls done with what was entered and whether the user
// confirmed (false on Cancel/Esc).
func (t *tui) showSecretPrompt(title, label string, done func(value string, ok bool)) {
	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" " + title + " ").SetTitleAlign(tview.AlignCenter)

	field := tview.NewInputField().SetLabel(label).SetMaskCharacter('*').SetFieldWidth(40)
	form.AddFormItem(field)

	submit := func() {
		t.returnToMain()
		done(field.GetText(), true)
	}
	cancel := func() {
		t.returnToMain()
		done("", false)
	}

	form.AddButton("OK", submit)
	form.AddButton("Cancel", cancel)
	form.SetCancelFunc(cancel)

	t.app.SetRoot(form, true)
	t.app.SetFocus(form)
}

// collectPassphrases prompts for each encrypted identity file path in turn,
// then calls next with creds populated (or ok=false if any prompt was
// canceled).
func (t *tui) collectPassphrases(paths []string, creds domain.Credentials, next func(domain.Credentials, bool)) {
	if len(paths) == 0 {
		next(creds, true)
		return
	}
	path := paths[0]
	t.showSecretPrompt("Passphrase", path, func(value string, ok bool) {
		if !ok {
			next(creds, false)
			return
		}
		if creds.KeyPassphrases == nil {
			creds.KeyPassphrases = map[string]string{}
		}
		creds.KeyPassphrases[path] = value
		t.collectPassphrases(paths[1:], creds, next)
	})
}
