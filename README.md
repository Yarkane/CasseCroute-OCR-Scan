# Cassecroute OCR

Cassecroute OCR est un outil de scan des fichiers PDF scannés, via [PdfSandwich](http://www.tobias-elze.de/pdfsandwich/), qui offre une interface web basique pour lancer un scan, observer l'avancement, et télécharger le fichier nettoyé.

L'usage principal est le traitement de gros fichiers lourds pour la lecture sur de petits appareils comme des liseuses. Supprimer les images des fichiers PDF (fond, illustrations) et pré-traiter la lecture des caractères permet de largement accélérer le chargement des pages sur une grande variété de modèles de liseuses.

<img width="1842" height="899" alt="image" src="https://github.com/user-attachments/assets/3350fd3d-6398-4eed-b2f8-049b720f5605" />

Le traitement de [PdfSandwich](http://www.tobias-elze.de/pdfsandwich/) est simple :
- Lorsqu'un fichier est déposé dans `/data/input`, il est détecté et le lancement du processus est opéré par [autoocr](https://github.com/xperimental/autoocr).
- Passage sur le fichier PDF scanné via OCR, afin d'en détecter les éléments textuels et les retranscrire directement textuellement. Ce scan est réalisé grâce au projet [Tesseract](https://github.com/tesseract-ocr/tesseract) de reconnaissance de caractères.
- Un fichier PDF sera généré pour chaque page, replaçant le texte approximativement là où il se trouvait dans le document initial. Elles seront ensuite fusionnées.
- La plupart des illustrations disparaissent. Cependant, certains schémas peuvent être conservés après le traitement.
- Vous trouverez le résultat du traitement dans `/data/output`. Il sera également téléchargeable directement via l'interface.

Aperçu de pages traitées de l'ouvrage "Early Netherlandish Painting" par Erwin Panofsky :

<img width="550" height="750" alt="image" src="https://github.com/user-attachments/assets/e1e7311b-c932-4911-b99c-85c9855367b6" />
<img width="550" height="750" alt="image" src="https://github.com/user-attachments/assets/186a011b-1745-46b1-9d58-11c2adb0b0fa" />

## Installation (Linux)

Votre machine linux doit avoir les packages suivants :
`pdfsandwich, poppler-utils, tesseract-ocr, tesseract-ocr-fra, ghostscript`

`tesseract-ocr-fra` correspond au paquet de reconnaissance de caractères dans des contextes français. Vous devez également installer tous les paquets de reconnaissance de caractères dans les contextes des langues qui vous intéressent (`tesseract-ocr-eng`, `tesseract-ocr-deu`, `tesseract-ocr-ita` ...) et spécifier ces contextes dans le paramètre "language".
Être précis sur les langages utilisés permet de largement accélérer le processus de reconnaissance.

Par la suite, il vous faut permettre dans la policy de sécurité ImageMagick le traitement des fichiers .pdf :
`cp "_contrib/policy.xml" /etc/ImageMagick-6/policy.xml`

## Installation (Yunohost)

Un package d'installation Yunohost existe à [ce repository](https://github.com/Yarkane/CasseCroute-OCR-Scan_ynh).

## Usage

- Build le projet Go
- Lancer l'exécutable `autoocr` avec les différents paramètres
```plain
Usage of autoocr:
      --port                  Port for the web server. (default=8080)
      --delay duration        Processing delay after receiving watch events. (default 5s)
  -i, --input string          Directory to use for input. (default "input")
      --keep-original         Keep backup of original file. (default true)
      --languages string      OCR Languages to use. (default "deu+eng")
      --log-format string     Logging format to use. (default "plain")
      --log-level string      Logging level to show. (default "info")
  -o, --output string         Directory to use for output. (default "output")
      --pdf-sandwich string   Path to pdfsandwich utility. (default "pdfsandwich")
```

## Remerciements
Merci à [xperimental](https://github.com/xperimental) pour le projet [autoocr](https://github.com/xperimental/autoocr) qui aura servi de base à ce projet.

Merci aux projets open source `pdfsandwich` ([voir plus](http://www.tobias-elze.de/pdfsandwich/)), `tesseract` ([voir plus](https://github.com/tesseract-ocr/tesseract)) et `ImageMagick` ([voir plus](https://imagemagick.org/)) qui ont fourni les briques nécessaires. 

Merci à [LeBoufty](https://github.com/LeBoufty) pour le logo du site.
