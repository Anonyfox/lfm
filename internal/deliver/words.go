package deliver

import "strings"

// Wording is what a download page says around its files, per language;
// English is the fallback.
type Wording struct{ Heading, Thanks, Pending, Keep string }

var wordings = map[string]Wording{
	"en": {"Download: %s", "Thank you for your order. Download your files:", "Thank you for your order. Your files are being prepared and arrive at the email address you paid with. Keep this page: they will appear here too.", "Keep this page: updated versions appear here at the same address."},
	"de": {"Download: %s", "Danke für deine Bestellung. Hier lädst du deine Dateien herunter:", "Danke für deine Bestellung. Deine Dateien werden gerade vorbereitet und kommen an die E-Mail-Adresse, mit der du bezahlt hast. Behalte diese Seite: Sie erscheinen auch hier.", "Behalte diese Seite: Neue Versionen erscheinen unter derselben Adresse."},
	"fr": {"Téléchargement : %s", "Merci pour votre commande. Téléchargez vos fichiers :", "Merci pour votre commande. Vos fichiers sont en préparation et arriveront à l’adresse e-mail utilisée pour le paiement. Gardez cette page : ils y apparaîtront aussi.", "Gardez cette page : les mises à jour paraissent à la même adresse."},
	"es": {"Descarga: %s", "Gracias por tu pedido. Descarga tus archivos:", "Gracias por tu pedido. Tus archivos se están preparando y llegarán al correo con el que pagaste. Guarda esta página: también aparecerán aquí.", "Guarda esta página: las nuevas versiones aparecen en la misma dirección."},
}

// Words returns the download page wording for a language.
func Words(language string) Wording {
	base, _, _ := strings.Cut(strings.ToLower(language), "-")
	if w, ok := wordings[base]; ok {
		return w
	}
	return wordings["en"]
}
