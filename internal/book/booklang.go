package book

import (
	"regexp"
	"strings"

	"github.com/Anonyfox/lfm/internal/aitext"
)

// bookLang holds what the quality checks know about one language. Word
// lists match with Unicode word boundaries (aitext.Words): Go's \b knows
// ASCII only. Sentence-start patterns are anchored at the start.
type bookLang struct {
	signposts      *regexp.Regexp // announcing the chapter
	forwardBridge  *regexp.Regexp // the one signpost allowed in the last paragraph
	hardTells      *regexp.Regexp // phrases models over-use by large factors
	petWords       *regexp.Regexp // words current models repeat
	inline         *regexp.Regexp // corrective framing within a sentence
	negation       map[string]bool
	correction     *regexp.Regexp // sentence openings that correct the previous one
	pepHeading     *regexp.Regexp // closing sections of encouragement or recap
	recapOpener    *regexp.Regexp
	platitude      *regexp.Regexp
	vagueSource    *regexp.Regexp
	stockNames     *regexp.Regexp // case-sensitive
	composite      *regexp.Regexp // a case marked as invented or anonymized
	hedges         *regexp.Regexp
	transitions    *regexp.Regexp // formal sentence-initial connectors
	generalOpener  *regexp.Regexp
	imagine        *regexp.Regexp
	imperative     *regexp.Regexp // sentence-initial instructions
	valueHeading   *regexp.Regexp
	nominal        *regexp.Regexp // abstract-noun suffixes, on the word as written
	nominalStop    map[string]bool
	stopwords      map[string]bool
	properNouns    bool // capitalized words inside a sentence are names
	vowels         *regexp.Regexp
	nominalShare   float64
	specificsFloor float64
	readability    func(r readingStats) (score float64, low, high float64, label string)
	maxSentence    float64
}

// readingStats are the counts the readability formulas need.
type readingStats struct {
	words, sentences, syllables, letters, long, poly, mono, veryLong int
}

func (r readingStats) asl() float64 { return float64(r.words) / float64(r.sentences) }
func (r readingStats) asw() float64 { return float64(r.syllables) / float64(r.words) }

// start anchors an alternation at the start of a sentence, ending at a word
// boundary in any script.
func start(alt string) *regexp.Regexp {
	return regexp.MustCompile(`^(?:` + alt + `)(?:$|[^\p{L}\p{N}])`)
}

func wordSet(list string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(list) {
		out[w] = true
	}
	return out
}

func flesch(base, aslWeight, aswWeight float64) func(r readingStats) float64 {
	return func(r readingStats) float64 { return base - aslWeight*r.asl() - aswWeight*r.asw() }
}

var bookLangs = map[string]bookLang{
	"en": {
		signposts:     aitext.Words(`this chapter (?:explains|shows|covers|is about|will|walks|looks|gives|teaches)|in this chapter|by the end of this chapter|in the (?:next|following|last|previous) chapter|as we(?:'|’)ve seen|as we (?:saw|discussed|learned) (?:in|earlier)|(?:let(?:'|’)s|let us) (?:dive|explore|take a look|look at|begin|start|get started)|without further ado|in this (?:guide|post|article),? (?:we(?:'|’)ll|you(?:'|’)ll)|let(?:'|’)s break (?:it|this) down|here(?:'|’)s everything you need to know`),
		forwardBridge: aitext.Words(`in the next chapter`),
		hardTells:     aitext.Words(`delv(?:e|es|ed|ing)|tapestry|(?:a |is a |stands as a )?testament to|underscor(?:e|es|ed|ing)|showcas(?:e|es|ed|ing)|intricacies|multifaceted|groundbreaking|unwavering|embark(?:s|ed|ing)? on|plays? an? (?:crucial|pivotal|vital|key|central) role|ever-evolving|it(?:'|’)s (?:important|worth|crucial) to (?:note|remember|mention)|it is (?:important|worth|crucial) to (?:note|remember|mention)|(?:here(?:'|’)s|and that(?:'|’)s) why (?:this|it) matters|why this matters|this matters because|here(?:'|’)s the thing|make no mistake|pav(?:e|es|ed|ing) the way|sh(?:ed|eds|edding) (?:a )?light on|a deeper understanding|navigat(?:e|es|ing) the (?:complexities|challenges|world)|in the realm of|it(?:'|’)s worth noting|it is worth noting|at its core|unlock the (?:full )?power of`),
		petWords:      aitext.Words(`dependabl[ey]|reliabl[ey]|quietly|leverag(?:e|es|ed|ing)|seamless(?:ly)?|robust`),
		inline:        aitext.Words(`(?:is|are|was)(?:n(?:'|’)t| not) (?:just|only|about|really|simply)[^.!?]{1,60}[,;:]\s*(?:it(?:'|’)s|it is|they(?:'|’)re|but)|more than (?:a|an|just)[^.!?]{1,40}[,;]\s*(?:it(?:'|’)s|it is)|not because[^.!?]{1,80}but because`),
		negation:      wordSet("not never no isn't isn’t aren't aren’t wasn't wasn’t weren't weren’t don't don’t doesn't doesn’t didn't didn’t won't won’t can't can’t"),
		correction:    start(`It(?:'|’)s|It is|That(?:'|’)s|This is|They(?:'|’)re|Instead|Rather|What matters|The real|The point is|The problem is|The answer is`),
		pepHeading:    regexp.MustCompile(`(?i)^(?:a (?:word|note) (?:for|to) you|a final (?:word|thought|note)|final thoughts?|before you go|a note of encouragement|remember|key takeaways|takeaways|in summary|summary|conclusion|wrapping up|the bottom line)$`),
		recapOpener:   start(`In short|In summary|To sum up|Overall|Ultimately|In the end|The bottom line|Remember,|So,|All in all`),
		platitude:     aitext.Words(`you(?:'|’)ve got this|you can do (?:this|it)|trust the process|be (?:gentle|kind|patient) with yourself|your journey|you deserve|you(?:'|’)re not alone`),
		vagueSource:   aitext.Words(`experts? (?:say|says|argue|agree|recommend|suggest)|many (?:experts|studies|scientists)|it is (?:widely|generally|commonly) (?:known|accepted|believed|agreed)`),
		stockNames:    regexp.MustCompile(`(?:^|[^\p{L}])(?:Sarah|Emily|Emma|Maya|Jake|Elara|Lisa|John Doe|Jane Doe|Alex)(?:$|[^\p{L}])`),
		composite:     aitext.Words(`composite|names? (?:has been |have been )?changed|anonymi[sz]ed|not (?:his|her|their) real name`),
		hedges:        aitext.Words(`(?:may|might|could|can) (?:potentially|possibly|perhaps|often|sometimes|generally)|might be able to|(?:may|can|could|might) (?:help|provide|offer|support|enhance|contribute)`),
		transitions:   start(`Moreover|Furthermore|Additionally|In addition|Notably|Importantly|Crucially|Consequently|Thus|Hence`),
		generalOpener: start(`In today(?:'|’)s|In a world|Every(?:one|body)|Most people|We all|Many (?:people|of us)|When it comes to|Throughout history|Have you ever|Imagine|Picture this|There (?:is|are) (?:a|an|many|few|no)|[A-Z]\w+(?: \w+)? (?:is|are) (?:a|an|the|one of)|In an increasingly`),
		imagine:       start(`Imagine|Picture this`),
		imperative:    start(`Add|Take|Put|Pour|Mix|Stir|Feed|Write|Open|Set|Check|Measure|Weigh|Keep|Stop|Start|Use|Try|Make|Wait|Let|Move|Place|Cut|Fill|Mark|Note|Count|Ask|Tell|Say|Send|Call|Read|Look|Smell|Taste|Turn|Cover|Leave|Remove|Repeat|Choose|Pick|List|Plan|Book|Block|Schedule|Track|Test|Compare|Calculate|Multiply|Divide|Subtract|Do|Don(?:'|’)t|Never|Always`),
		valueHeading:  aitext.Words(`example|worked example|try this|exercise|checklist|worksheet|steps?|how to|template|script`),
		nominal:       regexp.MustCompile(`^[a-z]{3,}(?:tion|sion|ment|ness|ity|ance|ence|ism)s?$`),
		nominalStop:   wordSet("question questions mention moment moments business station nation comment comments element elements sentence sentences evidence science experience audience difference balance distance instance city cement apartment apartments department government management payment statement statements document equipment"),
		stopwords:     wordSet("the a an and or but of to in on at for with by from as is are was were be been it its this that these those you your we our they their he she his her i me my not no so if then than there here what which who how when also just only very can will would should could may might do does did have has had"),
		properNouns:   true,
		vowels:        regexp.MustCompile(`(?i)[aeiouy]+`),
		nominalShare:  0.035, specificsFloor: 6,
		readability: func(r readingStats) (float64, float64, float64, string) {
			return flesch(206.835, 1.015, 84.6)(r), 50, 85, "Flesch reading ease"
		},
		maxSentence: 24,
	},
	"de": {
		signposts:     aitext.Words(`(?:dieses|das) kapitel (?:zeigt|erklärt|behandelt|beschreibt|führt)|in diesem kapitel|am ende (?:dieses|des) kapitels|im (?:nächsten|folgenden|letzten|vorigen) kapitel|wie (?:wir|du) (?:in kapitel \d+ |bereits |schon |oben )?gesehen (?:haben|hast)|wie bereits erwähnt|lass(?:t)? uns (?:eintauchen|beginnen|anfangen|loslegen|einen blick)|ohne (?:große|lange) umschweife|in diesem (?:beitrag|artikel|ratgeber) (?:erfährst du|erfahren sie|zeigen wir|zeige ich)|schauen wir uns \p{L}+ (?:genauer|näher) an`),
		forwardBridge: aitext.Words(`im nächsten kapitel`),
		hardTells:     aitext.Words(`wandteppich|kaleidoskop|facettenreich\p{L}*|vielschichtig\p{L}*|von (?:entscheidender|zentraler) bedeutung|spielt eine (?:entscheidende|zentrale|wichtige|große|maßgebliche) rolle|es ist (?:wichtig|entscheidend|wesentlich) zu (?:beachten|betonen|erwähnen)|es lohnt sich zu erwähnen|im spannungsfeld|an der schnittstelle (?:von|zwischen)|(?:ein )?licht auf \p{L}+ (?:werfen|wirft)|dreh- und angelpunkt|ein zeugnis (?:für|von)|unterstreicht die bedeutung|warum (?:das|dies) wichtig ist|genau deshalb ist (?:das|es) wichtig|ein tieferes verständnis|der schlüssel (?:dazu|liegt)|hier kommt \p{L}+(?: \p{L}+)? ins spiel`),
		petWords:      aitext.Words(`zuverlässig\p{L}*|verlässlich\p{L}*|nahtlos\p{L}*|robust\p{L}*`),
		inline:        aitext.Words(`es geht nicht (?:um|darum)[^.!?]{1,60}es geht (?:um|darum)|mehr als (?:ein|eine|nur)[^.!?]{1,40}[,;]\s*(?:es ist|das ist)|nicht, weil[^.!?]{1,80}sondern weil|ist nicht (?:einfach|nur|bloß)[^.!?]{1,60}[,;]\s*(?:es|das|sie|er) ist`),
		negation:      wordSet("nicht kein keine keinen keinem keiner nie niemals"),
		correction:    start(`Es ist|Das ist|Sondern|Es geht|Entscheidend ist|Was zählt|Wichtig ist|Der Punkt ist|Das Problem ist|Die Antwort ist|Stattdessen`),
		pepHeading:    regexp.MustCompile(`(?i)^(?:zum schluss|zum abschluss|(?:eine )?(?:kleine )?ermutigung.*|ein letztes wort|fazit|zusammenfassung|das wichtigste in kürze|kurz zusammengefasst|auf den punkt)$`),
		recapOpener:   start(`Zusammenfassend|Kurz gesagt|Letztendlich|Unterm Strich|Am Ende|Denk daran|Alles in allem|Fassen wir zusammen`),
		platitude:     aitext.Words(`du schaffst das|sei (?:nicht so streng|geduldig|nachsichtig) mit dir|vertrau(?:e)? (?:dem prozess|dir)|du bist nicht allein`),
		vagueSource:   aitext.Words(`(?:fachleute|experten|wissenschaftler|forscher) (?:sagen|empfehlen|raten|haben herausgefunden)|es ist (?:allgemein |weithin )?(?:bekannt|anerkannt), dass|forschungen? (?:hat|haben) (?:gezeigt|ergeben)`),
		stockNames:    regexp.MustCompile(`(?:^|[^\p{L}])(?:Max Mustermann|Erika Mustermann|Anna|Lisa|Julia|Sophie|Laura|Jonas|Lukas|Tom)(?:$|[^\p{L}])`),
		composite:     aitext.Words(`namen? geändert|anonymisiert|zusammengesetzt|erfunden`),
		hedges:        aitext.Words(`(?:kann|können|könnte|könnten) (?:unter umständen|möglicherweise|eventuell|vielleicht|gegebenenfalls)|(?:kann|können|könnte) (?:dabei )?(?:helfen|unterstützen)|in der regel oft`),
		transitions:   start(`Darüber hinaus|Des Weiteren|Ferner|Zudem|Folglich|Zusätzlich|Infolgedessen|Nichtsdestotrotz`),
		generalOpener: start(`In der heutigen|In einer Welt|Jede[rsn]?|Die meisten|Wir alle|Viele (?:Menschen|von uns)|Wenn es um|Stell dir vor|Hast du dich (?:jemals|schon einmal)|Kennst du das|Es gibt (?:viele|kaum|keine|wenige)|(?:Ein|Eine|Der|Die|Das) \p{L}+ (?:ist|sind) (?:ein|eine|der|die|das|einer)`),
		imagine:       start(`Stell dir vor`),
		imperative:    start(`Gib|Nimm|Stell|Stelle|Lass|Lasse|Miss|Wieg|Wiege|Schau|Füttere|Fütter|Rühr|Rühre|Gieß|Gieße|Wirf|Halte|Halt|Schreib|Schreibe|Prüf|Prüfe|Warte|Wart|Mach|Mache|Setz|Setze|Leg|Lege|Heb|Kipp|Kippe|Teste|Notier|Notiere|Markier|Markiere|Probier|Probiere|Achte|Merk|Merke|Nutze|Verwende|Bring|Fang|Riech|Tu|Koste|Deck|Decke|Füll|Fülle|Streich|Zähl|Zähle|Frag|Frage|Sag|Plane|Rechne|Vergleiche|Wähle|Trag|Trage|Öffne|Schließ|Kratz|Kratze|Entsorge|Spül|Spüle|Mische|Misch|Verrühr|Verrühre`),
		valueHeading:  aitext.Words(`beispiel|übung|checkliste|schritte?|so geht\p{L}*|vorlage|heute abend|anleitung`),
		nominal:       regexp.MustCompile(`^[A-ZÄÖÜ][a-zäöüß]{3,}(?:ung|heit|keit|ion|ität|ismus|schaft)(?:en)?$`),
		nominalStop:   wordSet("Wohnung Zeitung Rechnung Meinung Mannschaft Richtung Ordnung Erfahrung Wirkung Lösung Hoffnung Heizung Mischung Fütterung Anleitung Zeitungen Wohnungen Gesellschaft Freundschaft Landschaft Wissenschaft Nation Station Portion Portionen Region Million Millionen Version Funktion Position Situation"),
		stopwords:     wordSet("der die das ein eine einer eines einem einen und oder aber von zu im in am an auf für mit bei aus als ist sind war waren sein es sich du dein deine dir dich wir uns ihr sie er nicht kein keine so wenn dann da hier was wer wie wann auch nur sehr kann wird würde soll könnte mag tun hat haben hatte den dem des"),
		vowels:        regexp.MustCompile(`(?i)(?:ei|ie|eu|äu|au|aa|ee|oo|[aeiouyäöü])+?`),
		nominalShare:  0.05, specificsFloor: 3,
		readability: func(r readingStats) (float64, float64, float64, string) {
			return 180 - r.asl() - 58.5*r.asw(), 45, 1000, "Lesbarkeit nach Amstad"
		},
		maxSentence: 20,
	},
	"fr": {
		signposts:     aitext.Words(`ce chapitre (?:explique|montre|présente|traite|aborde|va)|dans ce chapitre|à la fin de ce chapitre|au (?:prochain|chapitre suivant|chapitre précédent)|dans le (?:prochain|chapitre suivant|chapitre précédent)|comme nous l(?:'|’)avons vu|comme (?:mentionné|évoqué) (?:plus haut|précédemment)|plongeons(?:-nous)?|commençons par|sans plus attendre|dans (?:cet article|ce guide)`),
		forwardBridge: aitext.Words(`(?:au|dans le) (?:prochain chapitre|chapitre suivant)`),
		hardTells:     aitext.Words(`témoign(?:e|ent) de|joue un rôle (?:crucial|essentiel|clé|central|déterminant)|il est (?:important|essentiel|crucial) de (?:noter|souligner|rappeler)|un véritable (?:allié|atout|levier)|incontournable\p{L}*|au cœur de (?:la|nos|votre)|une tapisserie|un paysage en constante évolution|ouvre la voie|mettre en lumière|une compréhension plus profonde|naviguer (?:dans|à travers) les|il convient de noter|la clé réside dans|il est (?:essentiel|crucial) de comprendre`),
		petWords:      aitext.Words(`fiable\p{L}*|fluide\p{L}*|harmonieu\p{L}*|optimal\p{L}*|robuste\p{L}*`),
		inline:        aitext.Words(`il ne s(?:'|’)agit pas (?:seulement |simplement )?d[e'’][^.!?]{1,60}mais d[e'’]|ce n(?:'|’)est pas (?:seulement |juste |qu(?:'|’))?[^.!?]{1,40}[,;:]\s*c(?:'|’)est|bien plus qu(?:'|’)un[e]?[^.!?]{1,40}[,;:]\s*c(?:'|’)est|non pas parce que[^.!?]{1,80}mais parce que`),
		negation:      wordSet("pas jamais aucun aucune rien"),
		correction:    start(`C(?:'|’)est|Il s(?:'|’)agit|Ce qui compte|Au contraire|Plutôt|En réalité|Le vrai`),
		pepHeading:    regexp.MustCompile(`(?i)^(?:pour conclure|en conclusion|conclusion|un dernier mot|le mot de la fin|en résumé|à retenir|ce qu(?:'|’)il faut retenir|un petit encouragement.*)$`),
		recapOpener:   start(`En résumé|En somme|Bref|Pour conclure|En conclusion|Au final|Finalement|Retenez|Souvenez-vous|N(?:'|’)oubliez pas`),
		platitude:     aitext.Words(`vous pouvez (?:le )?faire|faites-vous confiance|vous n(?:'|’)êtes pas seul\p{L}*|chaque petit pas compte|soyez indulgent\p{L}* avec vous-même|tu peux (?:le )?faire|tu n(?:'|’)es pas seul\p{L}*`),
		vagueSource:   aitext.Words(`(?:les )?experts (?:affirment|recommandent|s(?:'|’)accordent)|de nombreuses études (?:montrent|prouvent)|les études (?:montrent|prouvent)|il est (?:bien |largement )?(?:connu|admis) que|selon les spécialistes`),
		stockNames:    regexp.MustCompile(`(?:^|[^\p{L}])(?:Marie|Sophie|Julie|Camille|Thomas|Lucas|Jean Dupont)(?:$|[^\p{L}])`),
		composite:     aitext.Words(`le prénom a été changé|prénom modifié|anonymisé\p{L}*|personnage composite|inventé\p{L}*`),
		hedges:        aitext.Words(`(?:peut|peuvent|pourrait|pourraient) (?:potentiellement|éventuellement|parfois)|(?:peut|pourrait) (?:aider|contribuer|favoriser) à`),
		transitions:   start(`De plus|En outre|Par ailleurs|Ainsi|Par conséquent|De surcroît|Notamment|Néanmoins`),
		generalOpener: start(`Dans le monde d(?:'|’)aujourd(?:'|’)hui|De nos jours|Tout le monde|La plupart des gens|Nous avons tous|Beaucoup de gens|Quand il s(?:'|’)agit de|Imaginez|Imagine|Vous êtes-vous déjà|Il existe (?:de nombreux|plusieurs|beaucoup)|(?:Le|La|Les|Un|Une) \p{L}+ (?:est|sont) (?:un|une|le|la|les)|À l(?:'|’)ère du numérique`),
		imagine:       start(`Imaginez|Imagine|Figurez-vous`),
		imperative:    start(`Ajoutez|Ajoute|Prenez|Prends|Mettez|Mets|Versez|Verse|Mélangez|Mélange|Notez|Note|Mesurez|Mesure|Pesez|Pèse|Gardez|Garde|Commencez|Commence|Utilisez|Utilise|Essayez|Essaie|Attendez|Attends|Placez|Place|Coupez|Coupe|Remplissez|Remplis|Comptez|Compte|Demandez|Demande|Dites|Dis|Lisez|Lis|Vérifiez|Vérifie|Choisissez|Choisis|Planifiez|Planifie|Calculez|Calcule|Faites|Fais|Évitez|Évite|Retirez|Retire|Répétez|Répète|Ne (?:\p{L}+ )?(?:pas|jamais)`),
		valueHeading:  aitext.Words(`exemple|exercice|liste de contrôle|check-list|étapes?|mode d(?:'|’)emploi|modèle|méthode|en pratique`),
		nominal:       regexp.MustCompile(`^\p{Ll}{3,}(?:tion|sion|ment|ité|ance|ence|isme)s?$`),
		nominalStop:   wordSet("moment moments question questions appartement gouvernement information informations nation station"),
		stopwords:     wordSet("le la les un une des de du et ou mais à au aux en dans sur pour par avec sans est sont était être a ont il elle ils elles on nous vous je tu ce cette ces qui que quoi ne pas plus se sa son ses leur leurs y"),
		properNouns:   true,
		vowels:        regexp.MustCompile(`(?i)(?:eau|au|ai|ei|oi|ou|eu|œu|[aeiouyàâäéèêëîïôöùûüÿœ])+?`),
		nominalShare:  0.04, specificsFloor: 6,
		readability: func(r readingStats) (float64, float64, float64, string) {
			return flesch(207, 1.015, 73.6)(r), 45, 1000, "lisibilité Kandel-Moles"
		},
		maxSentence: 25,
	},
	"es": {
		signposts:     aitext.Words(`este capítulo (?:explica|muestra|presenta|trata|aborda|va)|en este capítulo|al final de este capítulo|en el (?:próximo|siguiente|anterior) capítulo|como (?:ya )?(?:hemos visto|vimos)|como se mencionó (?:antes|anteriormente)|(?:vamos a )?sumerg(?:irnos|ámonos)|empecemos por|sin más preámbulos|a continuación,? veremos|en este artículo descubrirás|en las próximas líneas`),
		forwardBridge: aitext.Words(`en el (?:próximo|siguiente) capítulo`),
		hardTells:     aitext.Words(`es un testimonio de|desempeña un papel (?:crucial|fundamental|clave|esencial)|juega un papel (?:crucial|fundamental|clave|esencial)|es importante (?:destacar|señalar|tener en cuenta) que|un verdadero aliado|un panorama en constante evolución|allana el camino|arroja(?:r)? luz sobre|una comprensión más profunda|navegar (?:por )?(?:las complejidades|los desafíos)|en el ámbito de|la clave está en|es aquí donde entra en juego`),
		petWords:      aitext.Words(`fiable\p{L}*|confiable\p{L}*|robust\p{L}*|óptim\p{L}*|sin fisuras`),
		inline:        aitext.Words(`no se trata (?:solo |sólo |simplemente )?de[^.!?]{1,60}sino de|no es (?:solo |sólo |simplemente )?[^.!?]{1,40}[,;:]\s*es|más que un[a]?[^.!?]{1,40}[,;:]\s*es|no porque[^.!?]{1,80}sino porque`),
		negation:      wordSet("no nunca jamás ni tampoco"),
		correction:    start(`Es|Se trata|Lo que importa|Lo importante|Al contrario|En realidad|Sino`),
		pepHeading:    regexp.MustCompile(`(?i)^(?:para terminar|conclusión|en conclusión|una última palabra|en resumen|resumen|lo que debes recordar|un pequeño ánimo.*|ánimo.*)$`),
		recapOpener:   start(`En resumen|En definitiva|En conclusión|Para terminar|Al final|Recuerda|No olvides|En pocas palabras`),
		platitude:     aitext.Words(`tú puedes|puedes (?:hacerlo|lograrlo)|confía en el proceso|no estás sol[oa]|cada pequeño paso cuenta|sé amable contigo`),
		vagueSource:   aitext.Words(`(?:los )?expertos (?:afirman|recomiendan|coinciden)|numerosos estudios (?:muestran|demuestran)|los estudios (?:muestran|demuestran)|es (?:bien |ampliamente )?sabido que|según los especialistas`),
		stockNames:    regexp.MustCompile(`(?:^|[^\p{L}])(?:María|Laura|Lucía|Ana|Carlos|Juan Pérez|Pedro)(?:$|[^\p{L}])`),
		composite:     aitext.Words(`el nombre ha sido cambiado|nombre ficticio|anonimizad\p{L}*|caso compuesto|inventad\p{L}*`),
		hedges:        aitext.Words(`(?:puede|pueden|podría|podrían) (?:potencialmente|eventualmente|a veces)|(?:puede|podría) (?:ayudar|contribuir) a`),
		transitions:   start(`Además|Asimismo|Por otro lado|Por lo tanto|En consecuencia|Cabe destacar|No obstante|Igualmente`),
		generalOpener: start(`En el mundo actual|Hoy en día|En la actualidad|Todo el mundo|La mayoría de la gente|Todos hemos|Mucha gente|Cuando se trata de|Imagina|Imagínate|¿Alguna vez|Existen (?:muchos|varios|numerosos)|(?:El|La|Los|Las|Un|Una) \p{L}+ (?:es|son) (?:un|una|el|la|los|las)|En un mundo cada vez más|En la era digital`),
		imagine:       start(`Imagina|Imagínate|Imagine`),
		imperative:    start(`Añade|Añada|Toma|Tome|Pon|Ponga|Vierte|Mezcla|Mezcle|Anota|Anote|Mide|Mida|Pesa|Pese|Guarda|Guarde|Empieza|Empiece|Usa|Use|Prueba|Pruebe|Espera|Espere|Coloca|Coloque|Corta|Corte|Llena|Cuenta|Pregunta|Pregunte|Di|Lee|Revisa|Revise|Elige|Elija|Planifica|Calcula|Calcule|Haz|Haga|Evita|Evite|Quita|Repite|No \p{L}+`),
		valueHeading:  aitext.Words(`ejemplo|ejercicio|lista de comprobación|checklist|pasos?|cómo hacerlo|plantilla|método|en la práctica`),
		nominal:       regexp.MustCompile(`^\p{Ll}{3,}(?:ción|sión|miento|dad|ancia|encia|ismo)(?:es|s)?$`),
		nominalStop:   wordSet("ciudad ciudades información momento verdad edad mitad"),
		stopwords:     wordSet("el la los las un una unos unas de del y o pero a al en con sin por para es son era ser ha han se su sus lo le les que qué quien no más muy ya también yo tú él ella nosotros"),
		properNouns:   true,
		vowels:        regexp.MustCompile(`(?i)(?:[aeiouáéíóúü]*[iuy][aeiouáéíóú]+|[aeiouáéíóúü]+)`),
		nominalShare:  0.045, specificsFloor: 6,
		readability: func(r readingStats) (float64, float64, float64, string) {
			// Fernández-Huerta: syllables per 100 words and words per sentence
			return 206.84 - 0.60*100*r.asw() - 1.02*r.asl(), 50, 1000, "lecturabilidad Fernández-Huerta"
		},
		maxSentence: 26,
	},
	"it": {
		signposts:     aitext.Words(`questo capitolo (?:spiega|mostra|presenta|tratta|affronta)|in questo capitolo|alla fine di questo capitolo|nel (?:prossimo|seguente|precedente) capitolo|come abbiamo visto|come (?:già )?accennato|tuffiamoci|immergiamoci|iniziamo (?:da|con)|senza ulteriori indugi`),
		forwardBridge: aitext.Words(`nel (?:prossimo|seguente) capitolo`),
		hardTells:     aitext.Words(`testimonia(?:no)? (?:la|il|come)|svolge un ruolo (?:cruciale|fondamentale|chiave|essenziale)|gioca un ruolo (?:cruciale|fondamentale|chiave|essenziale)|è importante (?:notare|sottolineare|ricordare) che|un vero (?:alleato|punto di forza)|un panorama in continua evoluzione|apre la strada|fare luce su|una comprensione più profonda|navigare (?:tra|nelle) (?:complessità|sfide)|la chiave è|il segreto sta nel|è fondamentale capire che|ecco tutto quello che devi sapere`),
		petWords:      aitext.Words(`affidabil\p{L}*|robust\p{L}*|ottimal\p{L}*|fluid\p{L}*`),
		inline:        aitext.Words(`non si tratta (?:solo |soltanto |semplicemente )?di[^.!?]{1,60}ma di|non è (?:solo |soltanto )?[^.!?]{1,40}[,;:]\s*è|più di un[ao]?[^.!?]{1,40}[,;:]\s*è|non perché[^.!?]{1,80}ma perché`),
		negation:      wordSet("non mai né nessun nessuna"),
		correction:    start(`È|Si tratta|Quello che conta|Ciò che conta|Anzi|Piuttosto|In realtà`),
		pepHeading:    regexp.MustCompile(`(?i)^(?:per concludere|conclusione|in conclusione|un'ultima parola|in sintesi|riassumendo|da ricordare|un piccolo incoraggiamento.*)$`),
		recapOpener:   start(`In sintesi|In breve|Riassumendo|In conclusione|Per concludere|Alla fine|Ricorda|Non dimenticare|In poche parole|In definitiva`),
		platitude:     aitext.Words(`ce la puoi fare|puoi farcela|fidati del processo|non sei sol[oa]|ogni piccolo passo conta|sii gentile con te stess[oa]`),
		vagueSource:   aitext.Words(`(?:gli )?esperti (?:affermano|consigliano|concordano)|numerosi studi (?:dimostrano|mostrano)|gli studi (?:dimostrano|mostrano)|è (?:ben |ampiamente )?noto che|secondo gli specialisti`),
		stockNames:    regexp.MustCompile(`(?:^|[^\p{L}])(?:Giulia|Francesca|Sara|Marco|Luca|Mario Rossi)(?:$|[^\p{L}])`),
		composite:     aitext.Words(`il nome è stato cambiato|nome di fantasia|anonimizzat\p{L}*|caso composito|inventat\p{L}*`),
		hedges:        aitext.Words(`(?:può|possono|potrebbe|potrebbero) (?:potenzialmente|eventualmente|talvolta)|(?:può|potrebbe) (?:aiutare|contribuire) a`),
		transitions:   start(`Inoltre|In aggiunta|Pertanto|Di conseguenza|Tuttavia|Ciononostante|Altresì`),
		generalOpener: start(`Nel mondo di oggi|Al giorno d(?:'|’)oggi|Oggigiorno|Tutti|La maggior parte delle persone|Molte persone|Quando si tratta di|Immagina|Ti sei mai chiest[oa]|Esistono (?:molti|diversi|numerosi)|(?:Il|La|Lo|I|Gli|Le|Un|Una) \p{L}+ (?:è|sono) (?:un|una|il|la|lo|i|gli|le)`),
		imagine:       start(`Immagina|Immaginate`),
		imperative:    start(`Aggiungi|Aggiunga|Prendi|Prenda|Metti|Versa|Mescola|Annota|Misura|Pesa|Tieni|Inizia|Usa|Prova|Aspetta|Posiziona|Taglia|Riempi|Conta|Chiedi|Di(?:'|’)|Leggi|Controlla|Scegli|Pianifica|Calcola|Fai|Evita|Togli|Ripeti|Non \p{L}+`),
		valueHeading:  aitext.Words(`esempio|esercizio|lista di controllo|checklist|passaggi|passi|come fare|modello|metodo|in pratica`),
		nominal:       regexp.MustCompile(`^\p{Ll}{3,}(?:zione|zioni|sione|sioni|mento|menti|ità|anza|anze|enza|enze|ismo|ismi)$`),
		nominalStop:   wordSet("momento momenti città informazione informazioni appartamento"),
		stopwords:     wordSet("il lo la i gli le un uno una di del della dei delle e o ma a al in con senza per tra fra è sono era essere ha hanno si suo sua suoi che chi non più molto anche io tu lui lei noi voi"),
		properNouns:   true,
		vowels:        regexp.MustCompile(`(?i)(?:[aeiouàèéìòù]*[iu][aeouàèéòù]+|[aeiouàèéìòù]+)`),
		nominalShare:  0.045, specificsFloor: 6,
		readability: func(r readingStats) (float64, float64, float64, string) {
			// Gulpease works on letters, not syllables
			return 89 + (300*float64(r.sentences)-10*float64(r.letters))/float64(r.words), 40, 1000, "indice Gulpease"
		},
		maxSentence: 26,
	},
	"pt": {
		signposts:     aitext.Words(`este capítulo (?:explica|mostra|apresenta|trata|aborda|vai)|neste capítulo|no final deste capítulo|no (?:próximo|seguinte|capítulo anterior) capítulo|como (?:já )?vimos|como (?:já )?mencionado|vamos mergulhar|mergulhemos|comecemos por|sem mais delongas|neste artigo,? vamos (?:explorar|descobrir|ver)|ao longo deste artigo|neste guia completo`),
		forwardBridge: aitext.Words(`no (?:próximo|seguinte) capítulo`),
		hardTells:     aitext.Words(`é um testemunho de|desempenha um papel (?:crucial|fundamental|essencial|central)|é importante (?:destacar|ressaltar|notar|lembrar) que|um verdadeiro aliado|um cenário em constante evolução|abre caminho para|lançar luz sobre|uma compreensão mais profunda|navegar (?:pelas|nas) (?:complexidades|desafios)|tem um papel fundamental|uma ferramenta poderosa`),
		petWords:      aitext.Words(`confiáve\p{L}*|robust\p{L}*|otimiz\p{L}*|fluid\p{L}*`),
		inline:        aitext.Words(`não se trata (?:apenas |só |simplesmente )?de[^.!?]{1,60}mas (?:sim )?de|não é (?:só |apenas )?[^.!?]{1,40}[,;:]\s*é|mais do que um[a]?[^.!?]{1,40}[,;:]\s*é|não porque[^.!?]{1,80}mas porque`),
		negation:      wordSet("não nunca nem jamais nenhum nenhuma"),
		correction:    start(`É|Trata-se|O que importa|O importante|Pelo contrário|Na verdade|Mas sim`),
		pepHeading:    regexp.MustCompile(`(?i)^(?:para terminar|conclusão|em conclusão|uma última palavra|em resumo|resumo|o que deve lembrar|uma palavra de incentivo.*)$`),
		recapOpener:   start(`Em resumo|Em suma|Resumindo|Em conclusão|Para terminar|No fim|No final|Lembre-se|Lembra-te|Não se esqueça`),
		platitude:     aitext.Words(`você consegue|tu consegues|confie no processo|não está sozinh[oa]|não estás sozinh[oa]|cada pequeno passo conta|seja gentil consigo`),
		vagueSource:   aitext.Words(`(?:os )?especialistas (?:afirmam|recomendam|concordam)|diversos estudos (?:mostram|demonstram)|os estudos (?:mostram|demonstram)|é (?:bem |amplamente )?sabido que|segundo especialistas`),
		stockNames:    regexp.MustCompile(`(?:^|[^\p{L}])(?:Maria|Ana|Joana|João|Pedro|Fulano)(?:$|[^\p{L}])`),
		composite:     aitext.Words(`o nome foi alterado|nome fictício|anonimizad\p{L}*|caso composto|inventad\p{L}*`),
		hedges:        aitext.Words(`(?:pode|podem|poderia|poderiam) (?:potencialmente|eventualmente|por vezes)|(?:pode|poderia) (?:ajudar|contribuir) (?:a|para)`),
		transitions:   start(`Além disso|Adicionalmente|Por outro lado|Portanto|Consequentemente|Contudo|Todavia|Ademais`),
		generalOpener: start(`No mundo (?:de hoje|atual)|Hoje em dia|Atualmente|Toda a gente|Todo mundo|A maioria das pessoas|Muitas pessoas|Quando se trata de|Imagine|Imagina|Já (?:alguma vez )?(?:se|te) perguntou|Existem (?:muitos|vários|diversos)|(?:O|A|Os|As|Um|Uma) \p{L}+ (?:é|são) (?:um|uma|o|a|os|as)|Em um mundo cada vez mais|Num mundo cada vez mais`),
		imagine:       start(`Imagine|Imagina`),
		imperative:    start(`Adicione|Adiciona|Pegue|Pega|Coloque|Coloca|Despeje|Misture|Mistura|Anote|Anota|Meça|Mede|Pese|Pesa|Guarde|Guarda|Comece|Começa|Use|Usa|Experimente|Experimenta|Espere|Espera|Corte|Corta|Encha|Conte|Conta|Pergunte|Diga|Leia|Verifique|Verifica|Escolha|Escolhe|Planeie|Planeje|Calcule|Faça|Faz|Evite|Evita|Retire|Repita|Não \p{L}+`),
		valueHeading:  aitext.Words(`exemplo|exercício|lista de verificação|checklist|passos?|como fazer|modelo|método|na prática`),
		nominal:       regexp.MustCompile(`^\p{Ll}{3,}(?:ção|ções|são|sões|mento|mentos|dade|dades|ância|ência|ismo|ismos)$`),
		nominalStop:   wordSet("cidade cidades momento momentos informação verdade metade"),
		stopwords:     wordSet("o a os as um uma de do da dos das e ou mas em no na nos nas com sem por para é são era ser foi tem têm se seu sua que quem não mais muito também eu tu ele ela nós vocês você"),
		properNouns:   true,
		vowels:        regexp.MustCompile(`(?i)(?:ão|õe|ãe|[aeiouáéíóúâêôãõ]*[iu][aeouáéóâêôãõ]+|[aeiouáéíóúâêôãõ]+)`),
		nominalShare:  0.045, specificsFloor: 6,
		readability: func(r readingStats) (float64, float64, float64, string) {
			// Flesch adapted to Portuguese (Martins et al. 1996)
			return flesch(248.835, 1.015, 84.6)(r), 50, 1000, "Flesch (Martins)"
		},
		maxSentence: 26,
	},
	"nl": {
		signposts:     aitext.Words(`dit hoofdstuk (?:legt uit|laat zien|gaat over|behandelt|beschrijft)|in dit hoofdstuk|aan het einde van dit hoofdstuk|in het (?:volgende|vorige) hoofdstuk|zoals we (?:al )?(?:hebben )?gezien|zoals eerder (?:vermeld|genoemd)|laten we (?:erin )?duiken|laten we beginnen|zonder verder oponthoud|in deze (?:gids|blogpost)|in dit artikel lees je|laten we eens kijken`),
		forwardBridge: aitext.Words(`in het volgende hoofdstuk`),
		hardTells:     aitext.Words(`getuigt van|speelt een (?:cruciale|essentiële|sleutel|centrale|belangrijke) rol|het is belangrijk om (?:op te merken|te benadrukken|te onthouden)|een ware bondgenoot|een voortdurend veranderend landschap|baant de weg|licht werpen op|een dieper begrip|navigeren door de (?:complexiteit|uitdagingen)|het mooie is|de sleutel tot succes|naar een hoger niveau tillen`),
		petWords:      aitext.Words(`betrouwba\p{L}*|naadloo\p{L}*|robuust\p{L}*|optimaa\p{L}*`),
		inline:        aitext.Words(`het gaat niet (?:alleen |enkel |zozeer )?om[^.!?]{1,60}maar om|het is niet (?:alleen |zomaar )?[^.!?]{1,40}[,;:]\s*het is|meer dan een[^.!?]{1,40}[,;:]\s*het is|niet omdat[^.!?]{1,80}maar omdat`),
		negation:      wordSet("niet geen nooit niemand niets"),
		correction:    start(`Het is|Het gaat|Wat telt|Waar het om gaat|Integendeel|In werkelijkheid|Juist`),
		pepHeading:    regexp.MustCompile(`(?i)^(?:tot slot|tot besluit|conclusie|een laatste woord|samenvatting|samengevat|om te onthouden|een kleine aanmoediging.*)$`),
		recapOpener:   start(`Kortom|Samengevat|Samenvattend|Tot slot|Uiteindelijk|Onthoud|Vergeet niet|Al met al`),
		platitude:     aitext.Words(`je kunt het|jij kunt dit|vertrouw op het proces|je staat er niet alleen voor|elke kleine stap telt|wees mild voor jezelf`),
		vagueSource:   aitext.Words(`(?:de )?experts (?:zeggen|raden aan|zijn het erover eens)|talloze studies (?:tonen|laten zien)|onderzoek (?:toont aan|laat zien)|het is (?:algemeen )?bekend dat|volgens deskundigen`),
		stockNames:    regexp.MustCompile(`(?:^|[^\p{L}])(?:Sanne|Lisa|Emma|Daan|Jan Jansen|Piet)(?:$|[^\p{L}])`),
		composite:     aitext.Words(`de naam is veranderd|gefingeerde naam|geanonimiseerd|samengesteld (?:voorbeeld|geval)|verzonnen`),
		hedges:        aitext.Words(`(?:kan|kunnen|zou|zouden) (?:mogelijk|eventueel|soms) (?:kunnen )?|(?:kan|kunnen) (?:helpen|bijdragen)`),
		transitions:   start(`Bovendien|Daarnaast|Verder|Derhalve|Dientengevolge|Echter|Niettemin|Tevens`),
		generalOpener: start(`In de wereld van vandaag|Tegenwoordig|Iedereen|De meeste mensen|We hebben allemaal|Veel mensen|Als het gaat om|Stel je voor|Heb je je ooit afgevraagd|Er zijn (?:veel|talloze|verschillende)|(?:De|Het|Een) \p{L}+ (?:is|zijn) (?:een|de|het)|In een tijd waarin`),
		imagine:       start(`Stel je voor|Stelt u zich voor`),
		imperative:    start(`Voeg|Neem|Zet|Leg|Giet|Meng|Roer|Noteer|Meet|Weeg|Houd|Hou|Begin|Gebruik|Probeer|Wacht|Plaats|Snij|Snijd|Vul|Tel|Vraag|Zeg|Lees|Controleer|Kies|Plan|Bereken|Doe|Vermijd|Haal|Herhaal|Maak|Schrijf|Kijk|Ruik|Proef|Geen \p{L}+|Niet \p{L}+`),
		valueHeading:  aitext.Words(`voorbeeld|oefening|checklist|stappen|stap voor stap|zo doe je dat|sjabloon|methode|in de praktijk`),
		nominal:       regexp.MustCompile(`^\p{Ll}{3,}(?:ing|ingen|heid|heden|tie|ties|teit|iteit|isme|ismen|schap)$`),
		nominalStop:   wordSet("ervaring woning regering richting kring opleiding vereniging"),
		stopwords:     wordSet("de het een en of maar van te in op aan voor met door uit als is zijn was waren ben bent heeft hebben had er dat die dit deze wie wat hoe waar niet geen ook nog wel al zo je jij u we wij ze zij hij ik"),
		properNouns:   true,
		vowels:        regexp.MustCompile(`(?i)(?:aa|ee|oo|uu|ie|oe|ui|ei|ij|eu|ou|au|[aeiouyëéèïö])+?`),
		nominalShare:  0.05, specificsFloor: 6,
		readability: func(r readingStats) (float64, float64, float64, string) {
			return flesch(206.835, 0.93, 77)(r), 45, 1000, "Flesch-Douma"
		},
		maxSentence: 22,
	},
	"pl": {
		signposts:     aitext.Words(`ten rozdział (?:wyjaśnia|pokazuje|omawia|przedstawia)|w tym rozdziale|na końcu tego rozdziału|w (?:następnym|kolejnym|poprzednim) rozdziale|jak (?:już )?widzieliśmy|jak wspomniano (?:wcześniej|wyżej)|zanurzmy się|zacznijmy od|bez zbędnych wstępów|poniżej przedstawiamy|przyjrzyjmy się`),
		forwardBridge: aitext.Words(`w (?:następnym|kolejnym) rozdziale`),
		hardTells:     aitext.Words(`świadczy o|odgrywa (?:kluczową|istotną|ważną|centralną) rolę|warto (?:zauważyć|podkreślić|pamiętać), że|prawdziwy sprzymierzeniec|stale zmieniający się krajobraz|toruje drogę|rzuca(?:ć)? światło na|głębsze zrozumienie|nawigować (?:po|przez) (?:zawiłościach|wyzwaniach)|kluczowe jest to, że|warto mieć na uwadze|ma ogromne znaczenie`),
		petWords:      aitext.Words(`niezawodn\p{L}*|bezproblemow\p{L}*|optymaln\p{L}*|solidn\p{L}*`),
		inline:        aitext.Words(`nie chodzi (?:tylko |wyłącznie |jedynie )?o[^.!?]{1,60}(?:ale|lecz) o|to nie (?:tylko |jedynie )?[^.!?]{1,40}[,;:]\s*to|to coś więcej niż|nie dlatego, że[^.!?]{1,80}(?:ale|lecz) dlatego, że`),
		negation:      wordSet("nie nigdy ani żaden żadna nic"),
		correction:    start(`To jest|Chodzi|Liczy się|Wręcz przeciwnie|W rzeczywistości|Raczej|Lecz`),
		pepHeading:    regexp.MustCompile(`(?i)^(?:na koniec|podsumowanie|zakończenie|ostatnie słowo|w skrócie|warto zapamiętać|słowo otuchy.*)$`),
		recapOpener:   start(`Podsumowując|Krótko mówiąc|Reasumując|Na koniec|Ostatecznie|Pamiętaj|Nie zapominaj|Słowem`),
		platitude:     aitext.Words(`dasz radę|uda ci się|zaufaj procesowi|nie jesteś sam[a]?|każdy mały krok się liczy|bądź dla siebie wyrozumiał[ya]`),
		vagueSource:   aitext.Words(`(?:eksperci|specjaliści) (?:twierdzą|zalecają|są zgodni)|liczne badania (?:pokazują|dowodzą)|badania (?:pokazują|dowodzą)|powszechnie wiadomo, że|według specjalistów`),
		stockNames:    regexp.MustCompile(`(?:^|[^\p{L}])(?:Anna|Kasia|Ola|Marek|Jan Kowalski|Piotr)(?:$|[^\p{L}])`),
		composite:     aitext.Words(`imię zostało zmienione|imię zmienione|zanonimizowan\p{L}*|przykład złożony|zmyślon\p{L}*`),
		hedges:        aitext.Words(`(?:może|mogą|mógłby|mogłoby) (?:potencjalnie|ewentualnie|czasem)|(?:może|mógłby) (?:pomóc|przyczynić się)`),
		transitions:   start(`Ponadto|Co więcej|Dodatkowo|W związku z tym|W konsekwencji|Niemniej|Jednakże|Zatem`),
		generalOpener: start(`W dzisiejszym świecie|Obecnie|W dzisiejszych czasach|Każdy|Większość ludzi|Wszyscy|Wiele osób|Jeśli chodzi o|Wyobraź sobie|Czy kiedykolwiek|Istnieje wiele|\p{L}+ (?:to|jest) (?:jeden|jedna|jedno)|W erze cyfrowej`),
		imagine:       start(`Wyobraź sobie|Wyobraźcie sobie`),
		imperative:    start(`Dodaj|Weź|Połóż|Postaw|Wlej|Wymieszaj|Zanotuj|Zapisz|Zmierz|Zważ|Trzymaj|Zacznij|Użyj|Spróbuj|Poczekaj|Umieść|Pokrój|Napełnij|Policz|Zapytaj|Powiedz|Przeczytaj|Sprawdź|Wybierz|Zaplanuj|Oblicz|Zrób|Unikaj|Usuń|Powtórz|Nie \p{L}+`),
		valueHeading:  aitext.Words(`przykład|ćwiczenie|lista kontrolna|checklista|kroki|krok po kroku|jak to zrobić|szablon|metoda|w praktyce`),
		nominal:       regexp.MustCompile(`^\p{Ll}{3,}(?:anie|enie|cja|cje|ość|ości|izm|izmy|stwo|stwa)$`),
		nominalStop:   wordSet("zdanie mieszkanie pytanie spotkanie jedzenie miasto informacja"),
		stopwords:     wordSet("i a w z na do od po o u za przez dla że to jest są był była było być się nie tak jak co kto gdzie kiedy już też ale lub czy ten ta te tego tej ja ty on ona my wy oni"),
		properNouns:   true,
		vowels:        regexp.MustCompile(`(?i)(?:ia|ie|io|iu|ią|ię|[aeiouyąęó])+?`),
		nominalShare:  0.05, specificsFloor: 6,
		readability: func(r readingStats) (float64, float64, float64, string) {
			// FOG-PL (Pisarek): words of four or more syllables count as hard
			fog := 0.4 * (r.asl() + 100*float64(r.veryLong)/float64(r.words))
			return -fog, -15, 1000, "FOG-PL (negated)"
		},
		maxSentence: 22,
	},
}

// bookLangOf returns the language data, English when lfm does not know it,
// and whether it is known.
func bookLangOf(language string) (bookLang, bool) {
	base, _, _ := strings.Cut(strings.ToLower(language), "-")
	if l, ok := bookLangs[base]; ok {
		return l, true
	}
	return bookLangs["en"], false
}
