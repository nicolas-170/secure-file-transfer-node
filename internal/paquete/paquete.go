// Package paquete arma y abre el paquete que viaja entre sedes.
//
// Al armarlo:
//  1. el archivo se codifica en Base64
//  2. se cifra con AES-256-GCM (clave derivada con Argon2id)
//  3. se calcula su hash SHA-256
//  4. todo se comprime en un ZIP junto al manifiesto
//
// Abrirlo es el camino inverso, verificando el hash antes de descifrar.
package paquete

import (
	"archive/zip"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	nombreManifiesto = "manifiesto.json"
	tamSalt          = 16 // sal para derivar la clave
	tamNonce         = 12 // tamaño que exige AES-GCM
	tamClave         = 32 // 32 bytes = AES-256
)

// Manifiesto acompaña al archivo cifrado y permite verificar en destino que
// llegó completo y sin modificaciones.
type Manifiesto struct {
	Nombre    string `json:"nombre"`
	Area      string `json:"area"`
	Fecha     string `json:"fecha"`
	Sede      string `json:"sede"`
	Algoritmo string `json:"algoritmo"`
	Hash      string `json:"hash_sha256"`
	Tamano    int    `json:"tamano_original"`
	Enviado   string `json:"enviado"`
}

// Crear lee el archivo y devuelve el ZIP listo para transmitir.
func Crear(ruta string, m Manifiesto, passphrase string) ([]byte, Manifiesto, error) {
	original, err := os.ReadFile(ruta)
	if err != nil {
		return nil, m, err
	}

	// 1. Base64: deja el contenido en caracteres imprimibles.
	codificado := []byte(base64.StdEncoding.EncodeToString(original))

	// 2. Cifrado simétrico.
	cifrado, err := cifrar(codificado, passphrase)
	if err != nil {
		return nil, m, err
	}

	// 3. Hash de lo que realmente viaja.
	m.Algoritmo = "AES-256-GCM"
	m.Hash = Hash(cifrado)
	m.Tamano = len(original)
	m.Enviado = time.Now().Format(time.RFC3339)

	// 4. ZIP con el archivo cifrado y el manifiesto.
	datosManifiesto, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, m, err
	}

	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	if err := agregar(z, m.Nombre+".enc", cifrado); err != nil {
		return nil, m, err
	}
	if err := agregar(z, nombreManifiesto, datosManifiesto); err != nil {
		return nil, m, err
	}
	if err := z.Close(); err != nil {
		return nil, m, err
	}

	return buf.Bytes(), m, nil
}

// Abrir descomprime, verifica el hash, descifra y decodifica.
// Devuelve el nombre original del archivo y su contenido.
func Abrir(datosZip []byte, passphrase string) (string, []byte, error) {
	z, err := zip.NewReader(bytes.NewReader(datosZip), int64(len(datosZip)))
	if err != nil {
		return "", nil, err
	}

	var cifrado, datosManifiesto []byte
	for _, f := range z.File {
		contenido, err := leer(f)
		if err != nil {
			return "", nil, err
		}
		if f.Name == nombreManifiesto {
			datosManifiesto = contenido
		} else {
			cifrado = contenido
		}
	}
	if cifrado == nil || datosManifiesto == nil {
		return "", nil, errors.New("el paquete no trae el archivo cifrado y su manifiesto")
	}

	var m Manifiesto
	if err := json.Unmarshal(datosManifiesto, &m); err != nil {
		return "", nil, err
	}

	// Integridad: el hash debe coincidir con el registrado en origen.
	if h := Hash(cifrado); h != m.Hash {
		return "", nil, fmt.Errorf("hash distinto: se esperaba %s y llegó %s", m.Hash[:12], h[:12])
	}

	codificado, err := descifrar(cifrado, passphrase)
	if err != nil {
		return "", nil, err
	}

	original, err := base64.StdEncoding.DecodeString(string(codificado))
	if err != nil {
		return "", nil, err
	}

	return m.Nombre, original, nil
}

// Hash devuelve el SHA-256 en hexadecimal. Sirve para comprobar que lo que
// llegó a la sede destino es idéntico a lo que salió de la principal.
func Hash(datos []byte) string {
	suma := sha256.Sum256(datos)
	return hex.EncodeToString(suma[:])
}

// cifrar protege los datos con AES-256-GCM.
// Resultado: salt (16) + nonce (12) + datos cifrados + etiqueta.
// La etiqueta de GCM delata cualquier modificación posterior del archivo.
func cifrar(datos []byte, passphrase string) ([]byte, error) {
	salt, err := aleatorio(tamSalt)
	if err != nil {
		return nil, err
	}
	// El nonce debe ser distinto en cada cifrado: repetirlo rompe GCM.
	nonce, err := aleatorio(tamNonce)
	if err != nil {
		return nil, err
	}

	gcm, err := nuevoGCM(passphrase, salt)
	if err != nil {
		return nil, err
	}

	salida := append(salt, nonce...)
	return append(salida, gcm.Seal(nil, nonce, datos, nil)...), nil
}

// descifrar deshace cifrar. Falla si la clave es incorrecta o los datos
// fueron alterados.
func descifrar(datos []byte, passphrase string) ([]byte, error) {
	if len(datos) < tamSalt+tamNonce {
		return nil, errors.New("datos cifrados incompletos")
	}

	salt := datos[:tamSalt]
	nonce := datos[tamSalt : tamSalt+tamNonce]

	gcm, err := nuevoGCM(passphrase, salt)
	if err != nil {
		return nil, err
	}

	claro, err := gcm.Open(nil, nonce, datos[tamSalt+tamNonce:], nil)
	if err != nil {
		return nil, errors.New("clave incorrecta o datos alterados")
	}
	return claro, nil
}

// nuevoGCM deriva la clave con Argon2id y prepara el cifrador.
// Usar la contraseña tal cual como clave sería inseguro; Argon2id encarece
// muchísimo los ataques de fuerza bruta.
func nuevoGCM(passphrase string, salt []byte) (cipher.AEAD, error) {
	clave := argon2.IDKey([]byte(passphrase), salt, 1, 64*1024, 4, tamClave)
	bloque, err := aes.NewCipher(clave)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(bloque)
}

func aleatorio(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := io.ReadFull(rand.Reader, b)
	return b, err
}

func agregar(z *zip.Writer, nombre string, contenido []byte) error {
	w, err := z.Create(nombre)
	if err != nil {
		return err
	}
	_, err = w.Write(contenido)
	return err
}

func leer(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
