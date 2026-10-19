package term

import "testing"

func TestSuggestFindsTheMeantWord(t *testing.T) {
	cmds := []string{"init", "compile", "serve", "rules", "skills", "version"}
	for word, want := range map[string]string{"comple": "compile", "compiel": "compile", "serv": "serve", "skils": "skills", "bogus": "", "x": ""} {
		if got := Suggest(word, cmds); got != want {
			t.Errorf("Suggest(%q) = %q, want %q", word, got, want)
		}
	}
	if got := Suggest("relase", []string{"release", "json"}); got != "release" {
		t.Errorf("flag %q", got)
	}
}
