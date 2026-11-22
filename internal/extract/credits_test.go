package extract

import (
	"reflect"
	"testing"
)

func TestParseCredits(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		want    []Credit
		company string
	}{
		{
			name: "run together, lowercase labels",
			text: "texte original de Lou Arnaud Mise en scène Bastien Coll avec Ana Ruiz, Mehdi Laroche et Jeanne Ollier",
			want: []Credit{
				{Name: "Lou Arnaud", Role: "author"}, {Name: "Bastien Coll", Role: "director"},
				{Name: "Ana Ruiz", Role: "performer"}, {Name: "Mehdi Laroche", Role: "performer"}, {Name: "Jeanne Ollier", Role: "performer"},
			},
		},
		{
			name: "one label per line, colons, characters",
			text: "D'après Anton Tchekhov\nTraduction : André Markowicz & Françoise Morvan\nMise en scène : Claire Vasseur\nAvec : Inès Bakari (Célimène), Thomas Lequeux (Alceste)\nLumières : Yann Le Goff\nProduction : Théâtre de la Cité",
			want: []Credit{
				{Name: "Anton Tchekhov", Role: "author"},
				{Name: "André Markowicz", Role: "translation"}, {Name: "Françoise Morvan", Role: "translation"},
				{Name: "Claire Vasseur", Role: "director"},
				{Name: "Inès Bakari", Role: "performer", Character: "Célimène"}, {Name: "Thomas Lequeux", Role: "performer", Character: "Alceste"},
				{Name: "Yann Le Goff", Role: "lighting"},
			},
		},
		{
			name: "label alone on its line",
			text: "Mise en scène\nPaul Dubreuil\nScénographie\nMia de la Tour",
			want: []Credit{{Name: "Paul Dubreuil", Role: "director"}, {Name: "Mia de la Tour", Role: "set_design"}},
		},
		{
			name: "prose is ignored",
			text: "Un spectacle drôle avec humour et tendresse. Son père le suit. La lumière tombe sur la scène.",
			want: nil,
		},
		{
			name:    "company in the cast",
			text:    "Avec : Compagnie des Lustres",
			want:    nil,
			company: "Compagnie des Lustres",
		},
		{
			name: "sentence after names",
			text: "Mise en scène Paul Dubreuil. Une farce en un acte.",
			want: []Credit{{Name: "Paul Dubreuil", Role: "director"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, company := ParseCredits(c.text)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("credits:\n got %+v\nwant %+v", got, c.want)
			}
			if company != c.company {
				t.Errorf("company = %q, want %q", company, c.company)
			}
		})
	}
}
