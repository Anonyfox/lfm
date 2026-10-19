package book

import "strings"

// Wording holds the few texts lfm writes into a book and its download page,
// per language; English is the fallback.
type Wording struct {
	Contents, Rights, CoverOf, Start, Copyright, Chapter string
	DownloadTitle, Thanks, Writing, Download, Keep       string
	EPUBNote, PDFNote                                    string
	Accessible                                           string // the EPUB's accessibility summary
}

var locales = map[string]Wording{
	"en": {
		Contents: "Contents", Rights: "All rights reserved.", CoverOf: "Cover of %s", Start: "Start", Copyright: "Copyright", Chapter: "Chapter %d",
		DownloadTitle: "Download: %s", Thanks: "Thank you for your order. Download your copy:",
		Writing:  "Thank you for your order. Your copy is being written right now and arrives at the email address you paid with as soon as it is finished. Keep this page: the files will appear here too.",
		Download: "Download", Keep: "Keep this page: updated editions appear here at the same address.",
		EPUBNote:   "For Apple Books, Kobo, Google Play Books and Kindle (send it to your Kindle address or use Send to Kindle).",
		PDFNote:    "For reading on a computer or printing.",
		Accessible: "This ebook reflows to the reader's font and size, has a navigable table of contents and structured headings, follows one reading order and describes every image in text. It has no known hazards.",
	},
	"de": {
		Contents: "Inhalt", Rights: "Alle Rechte vorbehalten.", CoverOf: "Cover von %s", Start: "Beginn", Copyright: "Impressum", Chapter: "Kapitel %d",
		DownloadTitle: "Download: %s", Thanks: "Danke für deine Bestellung. Hier lädst du dein Exemplar herunter:",
		Writing:  "Danke für deine Bestellung. Dein Exemplar wird gerade geschrieben und kommt an die E-Mail-Adresse, mit der du bezahlt hast, sobald es fertig ist. Behalte diese Seite: Die Dateien erscheinen auch hier.",
		Download: "Herunterladen", Keep: "Behalte diese Seite: Neue Auflagen erscheinen unter derselben Adresse.",
		EPUBNote:   "Für Apple Books, Kobo, Google Play Bücher und Kindle (an deine Kindle-Adresse senden oder „An Kindle senden“ nutzen).",
		PDFNote:    "Zum Lesen am Computer oder zum Ausdrucken.",
		Accessible: "Dieses E-Book passt sich Schriftart und -größe des Lesegeräts an, hat ein navigierbares Inhaltsverzeichnis und gegliederte Überschriften, folgt einer Lesereihenfolge und beschreibt jedes Bild in Text. Es birgt keine bekannten Gefahren.",
	},
	"fr": {
		Contents: "Sommaire", Rights: "Tous droits réservés.", CoverOf: "Couverture de %s", Start: "Début", Copyright: "Mentions légales", Chapter: "Chapitre %d",
		DownloadTitle: "Téléchargement : %s", Thanks: "Merci pour votre commande. Téléchargez votre exemplaire :",
		Writing:  "Merci pour votre commande. Votre exemplaire est en cours d’écriture et arrivera à l’adresse e-mail utilisée pour le paiement dès qu’il sera terminé. Gardez cette page : les fichiers y apparaîtront aussi.",
		Download: "Télécharger", Keep: "Gardez cette page : les nouvelles éditions paraissent à la même adresse.",
		EPUBNote:   "Pour Apple Books, Kobo, Google Play Livres et Kindle (envoyez-le à votre adresse Kindle ou utilisez Send to Kindle).",
		PDFNote:    "Pour lire sur ordinateur ou imprimer.",
		Accessible: "Ce livre numérique s’adapte à la police et à la taille choisies, offre une table des matières navigable et des titres structurés, suit un ordre de lecture unique et décrit chaque image par un texte. Il ne présente aucun danger connu.",
	},
	"es": {
		Contents: "Índice", Rights: "Todos los derechos reservados.", CoverOf: "Portada de %s", Start: "Inicio", Copyright: "Créditos", Chapter: "Capítulo %d",
		DownloadTitle: "Descarga: %s", Thanks: "Gracias por tu pedido. Descarga tu ejemplar:",
		Writing:  "Gracias por tu pedido. Tu ejemplar se está escribiendo ahora mismo y llegará al correo con el que pagaste en cuanto esté terminado. Guarda esta página: los archivos también aparecerán aquí.",
		Download: "Descargar", Keep: "Guarda esta página: las nuevas ediciones aparecen en la misma dirección.",
		EPUBNote:   "Para Apple Books, Kobo, Google Play Libros y Kindle (envíalo a tu dirección de Kindle o usa Send to Kindle).",
		PDFNote:    "Para leer en el ordenador o imprimir.",
		Accessible: "Este libro electrónico se adapta a la fuente y el tamaño del lector, tiene un índice navegable y encabezados estructurados, sigue un único orden de lectura y describe cada imagen con texto. No presenta riesgos conocidos.",
	},
}

// Words returns the texts lfm writes for a language.
func Words(language string) Wording {
	lang := strings.ToLower(language)
	if w, ok := locales[lang]; ok {
		return w
	}
	base, _, _ := strings.Cut(lang, "-")
	if w, ok := locales[base]; ok {
		return w
	}
	return locales["en"]
}
