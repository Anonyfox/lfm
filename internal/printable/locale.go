package printable

import "fmt"

// wording holds the few texts lfm writes for a printable: its download page,
// listing images and the facts in store descriptions. English is the fallback.
type wording struct {
	Heading, Thanks, Pending, Keep, PrintNote, DigitalNote string
	Page, Pages, Included, SizesLine, LinksLine            string
	PagesPerFile, LinksChecked, ToCut                      string
}

var locales = map[string]wording{
	"en": {
		Heading: "Download: %s", Thanks: "Thank you for your order. Download the files for your paper size:",
		Pending:   "Thank you for your order. Your files are being prepared and arrive at the email address you paid with. Keep this page: they will appear here too.",
		Keep:      "Keep this page: updated versions appear here at the same address. Print at 100% (actual size).",
		PrintNote: "Print at 100% (actual size).", DigitalNote: "For note apps such as GoodNotes or Notability; open in read-only mode to use the links.",
		Page: "page", Pages: "pages", Included: "Everything included: %d pages", SizesLine: "%d pages in every file", LinksLine: "%d links, all checked",
		PagesPerFile: "%d pages per file, in %s", LinksChecked: "%d internal links, every one checked", ToCut: "%d pages to cut out",
	},
	"de": {
		Heading: "Download: %s", Thanks: "Danke für deine Bestellung. Lade die Dateien für dein Papierformat herunter:",
		Pending:   "Danke für deine Bestellung. Deine Dateien werden gerade vorbereitet und kommen an die E-Mail-Adresse, mit der du bezahlt hast. Behalte diese Seite: Sie erscheinen auch hier.",
		Keep:      "Behalte diese Seite: Neue Versionen erscheinen unter derselben Adresse. Drucke in Originalgröße (100 %).",
		PrintNote: "In Originalgröße (100 %) drucken.", DigitalNote: "Für Notiz-Apps wie GoodNotes oder Notability; im Lesemodus öffnen, damit die Links funktionieren.",
		Page: "Seite", Pages: "Seiten", Included: "Alles dabei: %d Seiten", SizesLine: "%d Seiten in jeder Datei", LinksLine: "%d Links, alle geprüft",
		PagesPerFile: "%d Seiten pro Datei, in %s", LinksChecked: "%d interne Links, jeder einzelne geprüft", ToCut: "%d Seiten zum Ausschneiden",
	},
	"fr": {
		Heading: "Téléchargement : %s", Thanks: "Merci pour votre commande. Téléchargez les fichiers de votre format de papier :",
		Pending:   "Merci pour votre commande. Vos fichiers sont en préparation et arriveront à l’adresse e-mail utilisée pour le paiement. Gardez cette page : ils y apparaîtront aussi.",
		Keep:      "Gardez cette page : les mises à jour paraissent à la même adresse. Imprimez à 100 % (taille réelle).",
		PrintNote: "Imprimer à 100 % (taille réelle).", DigitalNote: "Pour les applications de notes comme GoodNotes ou Notability ; ouvrez en lecture seule pour utiliser les liens.",
		Page: "page", Pages: "pages", Included: "Tout est inclus : %d pages", SizesLine: "%d pages dans chaque fichier", LinksLine: "%d liens, tous vérifiés",
		PagesPerFile: "%d pages par fichier, en %s", LinksChecked: "%d liens internes, tous vérifiés", ToCut: "%d pages à découper",
	},
	"es": {
		Heading: "Descarga: %s", Thanks: "Gracias por tu pedido. Descarga los archivos para tu tamaño de papel:",
		Pending:   "Gracias por tu pedido. Tus archivos se están preparando y llegarán al correo con el que pagaste. Guarda esta página: también aparecerán aquí.",
		Keep:      "Guarda esta página: las nuevas versiones aparecen en la misma dirección. Imprime al 100 % (tamaño real).",
		PrintNote: "Imprimir al 100 % (tamaño real).", DigitalNote: "Para apps de notas como GoodNotes o Notability; ábrelo en modo lectura para usar los enlaces.",
		Page: "página", Pages: "páginas", Included: "Todo incluido: %d páginas", SizesLine: "%d páginas en cada archivo", LinksLine: "%d enlaces, todos comprobados",
		PagesPerFile: "%d páginas por archivo, en %s", LinksChecked: "%d enlaces internos, todos comprobados", ToCut: "%d páginas para recortar",
	},
}

func wordsFor(language string) wording {
	if w, ok := locales[base(language)]; ok {
		return w
	}
	return locales["en"]
}

// count is "1 page" or "n pages".
func (w wording) count(n int) string {
	if n == 1 {
		return "1 " + w.Page
	}
	return fmt.Sprintf("%d %s", n, w.Pages)
}
