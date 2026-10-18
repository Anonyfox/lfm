package aitext

import (
	"regexp"
	"strings"
	"unicode"
)

// Words compiles a case-insensitive alternation of words and phrases that
// matches only at word boundaries in any script. Go's \b knows ASCII only, so
// "\bübung\b" never matches; this boundary is any character that is not a
// letter or digit. Matches include the boundary characters; Trim removes them.
func Words(alternation string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])(?:` + alternation + `)(?:$|[^\p{L}\p{N}])`)
}

// Trim removes the boundary characters around a match of a Words pattern.
func Trim(match string) string {
	return strings.TrimFunc(match, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}

// CountWords counts the matches of a Words pattern, including adjacent ones
// that share a boundary character.
func CountWords(re *regexp.Regexp, s string) int {
	n := 0
	for {
		loc := re.FindStringIndex(s)
		if loc == nil {
			return n
		}
		n++
		// continue at the boundary character, so a following match can use it
		next := loc[1]
		if next > loc[0]+1 && next <= len(s) {
			next--
		}
		s = s[next:]
	}
}

// language is what the estimate knows about one language beyond its
// language-agnostic features.
type language struct {
	fillers    map[string]bool // hedges, fillers and particles of people writing
	habits     *regexp.Regexp  // words and phrases current models over-use
	gaps       *regexp.Regexp  // constructions with a gap: "it's not X, it's Y"
	negation   map[string]bool // words that negate a sentence
	correction *regexp.Regexp  // sentence openings that correct the previous one
	participle *regexp.Regexp  // a participle clause after a comma (Reinhart et al. 2025)
	nominal    *regexp.Regexp  // abstract-noun suffixes, on lowercased words
	person     map[string]bool // first and second person
}

func set(words string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(words) {
		out[w] = true
	}
	return out
}

// languages holds the western languages lfm knows. The lists were drafted
// from the literature (Kobak et al. 2025, Juzek and Ward 2025, Wikipedia's
// Signs of AI writing, Graphite 2026) and checked by native-language review;
// the models weigh them from data, so a list item that people use too only
// counts as much as the data says.
var languages = map[string]language{
	"en": {
		fillers:    set("just really very actually think know would could might maybe probably pretty kind sort stuff things lot i me my i'm i’m i've i’ve i'd i’d though anyway still even quite rather perhaps seem seems guess bit"),
		habits:     Words(`delv(?:e|es|ed|ing)|tapestry|testament to|underscor(?:e|es|ed|ing)|showcas(?:e|es|ed|ing)|multifaceted|pivotal|seamless(?:ly)?|robust|dependabl[ey]|quietly|leverag(?:e|es|ed|ing)|elevate[sd]?|unlock(?:s|ed|ing)?|empower(?:s|ed|ing)?|journey|game-changer|effortless(?:ly)?|tailored|streamline[sd]?|here(?:'|’)s (?:the thing|why|how|what)|the truth is|let(?:'|’)s (?:dive|explore|be honest)|in this (?:chapter|section|guide|post|article)|whether you(?:'|’)re|the good news|in short|crucial(?:ly)?|vital|essential|innovative|cutting-edge|emphasi[sz](?:e|es|ed|ing)|highlight(?:s|ed|ing)?|the result\?|the secret\?|ready to (?:start|get started|begin)\?|in today(?:'|’)s (?:fast-paced |digital |modern )?world|in this (?:guide|post|article),? (?:we(?:'|’)ll|you(?:'|’)ll)|let(?:'|’)s break (?:it|this) down|here(?:'|’)s everything you need to know|at its core|unlock the (?:full )?power of|it(?:'|’)s worth noting|in an increasingly`),
		gaps:       Words(`it(?:'|’)s not (?:just |only |about )?[^.!?]{1,40}[,;—–-] ?it(?:'|’)s|isn(?:'|’)t (?:just|about) [^.!?]{1,40}[.,;] ?it(?:'|’)s|more than (?:a|an|just) [^.!?]{1,30}[,;—–] ?it(?:'|’)s|not because [^.!?]{1,60} but because`),
		negation:   set("not never no isn't isn’t aren't aren’t don't don’t doesn't doesn’t won't won’t"),
		correction: regexp.MustCompile(`^(?:It(?:'|’)s|It is|That(?:'|’)s|This is|Instead|Rather|What matters|The real)\b`),
		participle: Words(`, (?:\p{L}+ing)`),
		nominal:    regexp.MustCompile(`^\p{Ll}+(?:tion|sion|ment|ness|ity|ance|ence|ism)s?$`),
		person:     set("i me my mine we us our you your yours i'm i’m we're we’re you're you’re i've i’ve"),
	},
	"de": {
		fillers:    set("doch ja mal halt eben eigentlich wohl schon irgendwie vielleicht ziemlich ich mir mich mein meine meinen denn nun gar etwa eher ganz bisschen sozusagen"),
		habits:     Words(`zuverlässig\p{L}*|verlässlich\p{L}*|nahtlos\p{L}*|maßgeschneidert\p{L}*|mühelos\p{L}*|ganzheitlich\p{L}*|vielschichtig\p{L}*|facettenreich\p{L}*|eintauchen|entfalten|reise|spielend leicht|im handumdrehen|optimal\p{L}*|in diesem (?:kapitel|abschnitt|artikel|beitrag|ratgeber)|egal,? ob du|ob (?:anfänger|einsteiger) oder (?:profi|fortgeschritten\p{L}*)|genau (?:hier|dafür|deshalb)|kurz gesagt|die gute nachricht|entscheidend\p{L}*|maßgeblich\p{L}*|essenziell\p{L}*|zukunftsweisend\p{L}*|wegweisend\p{L}*|bahnbrechend\p{L}*|unverzichtbar\p{L}*|innovativ\p{L}*|betont|betonen|hervorheben|hebt hervor|unterstreicht|unterstreichen|beleuchten|herzstück|das ergebnis\?|das geheimnis\?|bereit\?|in der heutigen (?:schnelllebigen |digitalen )?welt|in diesem (?:beitrag|artikel|ratgeber) (?:erfährst du|erfahren sie|zeigen wir)|auf einen blick|der schlüssel (?:dazu|liegt)|ins spiel kommt|kommt \p{L}+ ins spiel`),
		gaps:       Words(`es geht nicht (?:nur |darum |um )[^.!?]{1,60}(?:es geht|sondern)|mehr als (?:ein|eine|nur) [^.!?]{1,30}[,;—–] ?(?:es|das) ist`),
		negation:   set("nicht kein keine keinen keinem keiner nie niemals"),
		correction: regexp.MustCompile(`^(?:Es ist|Das ist|Sondern|Es geht|Entscheidend ist|Was zählt|Stattdessen)(?:$|[^\p{L}])`),
		participle: Words(`, (?:wobei|womit|wodurch|was (?:zeigt|bedeutet|unterstreicht))`),
		nominal:    regexp.MustCompile(`^\p{Ll}+(?:ung|heit|keit|ität|ismus|schaft)(?:en)?$`),
		person:     set("ich mir mich mein meine meinen meinem meiner wir uns unser unsere du dir dich dein deine deinen ihr euch euer sie"),
	},
	"fr": {
		fillers:    set("bon ben bah enfin bref franchement carrément tiens voilà hein justement plutôt d'ailleurs d’ailleurs j'avoue j’avoue je moi"),
		habits:     Words(`incontournable\p{L}*|révolutionn\p{L}*|sur[- ]mesure|sans effort|en toute (?:simplicité|sérénité|tranquillité)|plongeons|plongez|au cœur de|véritable allié\p{L}*|un jeu d'enfant|à portée de (?:main|clic)|n'attendez plus|que vous soyez|dans ce (?:chapitre|article|guide)|la bonne nouvelle|en somme|en résumé|il est important de (?:noter|souligner)|fiable\p{L}*|fluide\p{L}*|optimis\p{L}*|booster|crucial\p{L}*|essentiel\p{L}*|fondamental\p{L}*|primordial\p{L}*|indéniable\p{L}*|holistique\p{L}*|innovant\p{L}*|souligne\p{L}*|mettre en (?:lumière|évidence)|met en (?:lumière|évidence)|pierre angulaire|le résultat \?|le secret \?|prêt\p{L}* à vous lancer \?|dans le monde d(?:'|’)aujourd(?:'|’)hui|en constante évolution|dans (?:cet article|ce guide)|il convient de noter|la clé réside dans|à l(?:'|’)ère du numérique|découvrez comment`),
		gaps:       Words(`il ne s'agit pas (?:seulement |simplement )?d[e'][^.!?]{1,60}mais d[e']|ce n'est pas (?:seulement |juste |qu')?[^.!?]{1,40}[,;—–] ?c'est|bien plus qu['e] [^.!?]{1,40}[,;—–] ?c'est`),
		negation:   set("pas jamais aucun aucune ne n' plus"),
		correction: regexp.MustCompile(`^(?:C'est|C’est|Il s'agit|Il s’agit|Ce qui compte|En réalité|Au contraire|Plutôt)(?:$|[^\p{L}])`),
		participle: Words(`, (?:en )?\p{L}+ant`),
		nominal:    regexp.MustCompile(`^\p{Ll}+(?:tion|sion|ment|ité|ance|ence|isme)s?$`),
		person:     set("je j' me m' moi mon ma mes nous notre nos vous votre vos tu te toi ton ta tes"),
	},
	"es": {
		fillers:    set("pues bueno vamos vaya oye mira hombre claro igual encima total tampoco ojo digamos venga yo"),
		habits:     Words(`sin esfuerzo|a medida|imprescindible\p{L}*|sumérgete|potenci(?:a|ar|ará)\p{L}*|transform(?:a|ar)\p{L}* tu|viaje|aliado perfecto|de forma (?:sencilla|rápida y sencilla)|en un abrir y cerrar de ojos|ya sea que|tanto si eres|en este (?:capítulo|artículo|post)|la buena noticia|en resumen|es importante (?:destacar|señalar|tener en cuenta)|fiable\p{L}*|optimiz\p{L}*|sin complicaciones|crucial\p{L}*|fundamental\p{L}*|primordial\p{L}*|esencial\p{L}*|invaluable\p{L}*|holístic\p{L}*|innovador\p{L}*|vanguardia|sin fisuras|enfatiz\p{L}*|subraya\p{L}*|realza\p{L}*|cabe destacar|piedra angular|¿el resultado\?|¿el secreto\?|¿list[oa]s? para empezar\?|en el (?:mundo|panorama) actual|a continuación,? veremos|la clave está en|en la era digital|en un mundo cada vez más|descubre cómo`),
		gaps:       Words(`no se trata (?:solo |sólo )?de [^.!?]{1,60}sino de|no es (?:solo |sólo )?[^.!?]{1,40}[,;—–] ?es|más que un[a]? [^.!?]{1,40}[,;—–] ?es`),
		negation:   set("no nunca jamás ni tampoco"),
		correction: regexp.MustCompile(`^(?:Es |Se trata|Lo que importa|En realidad|Sino|Al contrario)`),
		participle: Words(`, \p{L}+(?:ando|iendo)`),
		nominal:    regexp.MustCompile(`^\p{Ll}+(?:ción|sión|miento|dad|ancia|encia|ismo)(?:es|s)?$`),
		person:     set("yo me mi mis conmigo nosotros nosotras nos nuestro nuestra tú te ti tu tus vosotros usted ustedes"),
	},
	"it": {
		fillers:    set("cioè insomma magari mica pure allora dunque comunque tipo praticamente appunto ecco beh boh mah figurati ormai eppure anzi piuttosto almeno proprio io"),
		habits:     Words(`senza sforzo|su misura|imprescindibil\p{L}*|immergiti|potenzia\p{L}*|viaggio|alleato perfetto|in modo semplice|in un batter d'occhio|che tu sia|in questo (?:capitolo|articolo|post)|la buona notizia|in sintesi|è importante (?:notare|sottolineare)|affidabil\p{L}*|ottimizza\p{L}*|senza complicazioni|fondamental\p{L}*|crucial\p{L}*|essenzial\p{L}*|prezios\p{L}*|impeccabil\p{L}*|mirat\p{L}*|approfondit\p{L}*|olistic\p{L}*|innovativ\p{L}*|sottolinea\p{L}*|evidenzia\p{L}*|valorizza\p{L}*|pilastro|un vero e proprio|addentriamoci|il risultato\?|il segreto\?|pront[oa] a iniziare\?|nel (?:mondo di oggi|panorama attuale)|la chiave è|il segreto sta nel|ecco tutto quello che devi sapere|scopri come|in poche parole`),
		gaps:       Words(`non si tratta (?:solo )?di [^.!?]{1,60}ma di|non è (?:solo |soltanto )?[^.!?]{1,40}[,;—–] ?è|più di un[ao]? [^.!?]{1,40}[,;—–] ?è`),
		negation:   set("non mai né nessun nessuna"),
		correction: regexp.MustCompile(`^(?:È |Si tratta|Quello che conta|In realtà|Piuttosto|Anzi)`),
		participle: Words(`, \p{L}+(?:ando|endo)`),
		nominal:    regexp.MustCompile(`^\p{Ll}+(?:zione|zioni|sione|mento|menti|ità|anza|enza|ismo)$`),
		person:     set("io me mi mio mia miei mie noi ci nostro nostra tu te ti tuo tua tuoi voi vi vostro"),
	},
	"pt": {
		fillers:    set("aliás enfim né pois então tipo olha aí afinal mesmo bom digamos convenhamos eu"),
		habits:     Words(`sem esforço|sob medida|imprescindível|mergulh(?:e|ar)|potencializ\p{L}*|jornada|aliado perfeito|de forma (?:simples|rápida e simples)|num piscar de olhos|seja você|neste (?:capítulo|artigo|post)|a boa notícia|em resumo|é importante (?:destacar|ressaltar|notar)|confiável|otimiz\p{L}*|sem complicações|crucial\p{L}*|fundamental\p{L}*|essencial\p{L}*|primordial\p{L}*|inestimáve\p{L}*|holístic\p{L}*|inovador\p{L}*|enfatiz\p{L}*|ressalt\p{L}*|salient\p{L}*|vale (?:ressaltar|destacar)|pilar|o resultado\?|o segredo\?|pront[oa] para começar\?|no (?:mundo|cenário) atual|neste artigo,? vamos|uma ferramenta poderosa|em um mundo cada vez mais|descubra como|neste guia completo`),
		gaps:       Words(`não se trata (?:apenas |só )?de [^.!?]{1,60}mas (?:sim )?de|não é (?:só |apenas )?[^.!?]{1,40}[,;—–] ?é|mais do que um[a]? [^.!?]{1,40}[,;—–] ?é`),
		negation:   set("não nunca nem jamais nenhum nenhuma"),
		correction: regexp.MustCompile(`^(?:É |Trata-se|O que importa|Na verdade|Mas sim|Pelo contrário)`),
		participle: Words(`, \p{L}+(?:ando|endo|indo)`),
		nominal:    regexp.MustCompile(`^\p{Ll}+(?:ção|ções|são|mento|mentos|dade|dades|ância|ência|ismo)$`),
		person:     set("eu me mim meu minha meus minhas nós nos nosso nossa tu te ti teu tua você vocês"),
	},
	"nl": {
		fillers:    set("toch wel gewoon eigenlijk even nou hoor zeg eens juist nogal best heus immers trouwens overigens sowieso tja kennelijk blijkbaar ergens alsnog helemaal echt ik"),
		habits:     Words(`naadloos|moeiteloos|op maat|onmisbaar|duik (?:in|erin)|reis|ideale partner|(?:snel en )?eenvoudig en snel|in een handomdraai|of je nu|in dit (?:hoofdstuk|artikel|blog)|het goede nieuws|kortom|het is belangrijk om (?:op te merken|te benadrukken)|betrouwba\p{L}*|optimaa\p{L}*|zorgeloos|cruciaal|cruciale|essentieel|essentiële|wezenlijk\p{L}*|holistisch\p{L}*|innovatie[fv]\p{L}*|veelzijdig\p{L}*|benadruk\p{L}*|onderstre\p{L}*|belicht\p{L}*|hoeksteen|sleutelrol|het resultaat\?|het geheim\?|klaar om te beginnen\?|in (?:de huidige wereld|het huidige landschap)|in deze (?:gids|blogpost)|het mooie is|naar een hoger niveau|in een tijd waarin|ontdek hoe`),
		gaps:       Words(`het gaat niet (?:alleen |enkel )?om [^.!?]{1,60}maar om|het is niet (?:alleen |zomaar )?[^.!?]{1,40}[,;—–] ?het is|meer dan een [^.!?]{1,40}[,;—–] ?het is`),
		negation:   set("niet geen nooit niemand"),
		correction: regexp.MustCompile(`^(?:Het is|Het gaat|Wat telt|In werkelijkheid|Integendeel)(?:$|[^\p{L}])`),
		participle: Words(`, \p{L}+end(?:e)?`),
		nominal:    regexp.MustCompile(`^\p{Ll}+(?:ing|ingen|heid|heden|tie|ties|iteit|isme)$`),
		person:     set("ik me mij mijn wij we ons onze jij je jou jouw jullie u uw"),
	},
	"pl": {
		fillers:    set("przecież chyba właśnie niby zresztą nawet raczej pewnie akurat ponoć podobno jakby wręcz skądinąd wiadomo cóż otóż owszem dopiero generalnie jakoś ja"),
		habits:     Words(`bezproblemow\p{L}*|bez wysiłku|na miarę|niezbędn\p{L}*|zanurz się|odkryj|podróż\p{L}*|idealn\p{L}* partner\p{L}*|w prosty sposób|w mgnieniu oka|niezależnie od tego,? czy|w tym (?:rozdziale|artykule|wpisie)|dobra wiadomość|podsumowując|warto (?:zauważyć|podkreślić)|niezawodn\p{L}*|optymaliz\p{L}*|kluczow\p{L}*|istotn\p{L}*|nieoceniony\p{L}*|kompleksow\p{L}*|holistyczn\p{L}*|innowacyjn\p{L}*|zaawansowan\p{L}*|podkreśl\p{L}*|uwypukl\p{L}*|zgłębia\p{L}*|filar\p{L}*|kamień węgielny|efekt\?|sekret\?|gotow[ya] (?:by|żeby) zacząć\?|w dzisiejszym (?:dynamicznym )?świecie|poniżej przedstawiamy|kluczowe jest to, że|w erze cyfrowej|przyjrzyjmy się|warto mieć na uwadze`),
		gaps:       Words(`nie chodzi (?:tylko |wyłącznie )?o [^.!?]{1,60}(?:ale|lecz) o|to nie (?:tylko |jedynie )?[^.!?]{1,40}[,;—–] ?to|to coś więcej niż`),
		negation:   set("nie nigdy ani żaden żadna"),
		correction: regexp.MustCompile(`^(?:To jest|Chodzi|Liczy się|W rzeczywistości|Wręcz przeciwnie|Lecz)(?:$|[^\p{L}])`),
		participle: Words(`, \p{L}+(?:ąc|wszy)`),
		nominal:    regexp.MustCompile(`^\p{Ll}+(?:anie|enie|ość|ości|acja|acje|izm)$`),
		person:     set("ja mnie mi mój moja moje my nas nam nasz nasza ty ciebie cię ci twój twoja wy was wasz"),
	},
}

// habits counts the model habits of a text: over-used words and phrases,
// constructions with a gap, and corrective framing across two sentences.
func habits(paragraphs []string, lang string) int {
	l, ok := languages[lang]
	if !ok {
		return 0
	}
	n := 0
	for _, p := range paragraphs {
		n += CountWords(l.habits, p) + CountWords(l.gaps, p)
		ss := sentences(p)
		for i := 1; i < len(ss); i++ {
			if !l.correction.MatchString(ss[i]) {
				continue
			}
			prev := words(ss[i-1])
			if len(prev) > 20 {
				continue
			}
			for _, w := range prev {
				if l.negation[strings.ToLower(w)] {
					n++
					break
				}
			}
		}
	}
	return n
}
