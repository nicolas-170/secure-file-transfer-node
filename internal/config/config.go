// Package config lee los parámetros de la línea de comandos.
// Lo que no se indique toma un valor por defecto.
package config

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config es la configuración ya resuelta de un envío.
type Config struct {
	Host    string // de --url
	Puerto  int    // de --url, o 22
	Usuario string // de --url

	Archivos []string // archivos a enviar, con los comodines ya expandidos
	Destino  string   // carpeta remota donde guardarlos ("" = usar las por defecto)

	KeyFile    string // clave privada SSH
	Password   string // contraseña SSH (variable de entorno)
	Passphrase string // clave del cifrado AES (variable de entorno)
	KnownHosts string // host keys conocidas

	DryRun       bool // valida sin cifrar ni conectarse
	Estricto     bool // aborta si un nombre no cumple la nomenclatura
	SinVerificar bool // acepta cualquier host key (solo laboratorio)
	SinColor     bool
}

// Lista permite repetir el flag --file varias veces.
type Lista []string

func (l *Lista) String() string     { return strings.Join(*l, ",") }
func (l *Lista) Set(v string) error { *l = append(*l, v); return nil }

// Parse define los flags, aplica los valores por defecto y expande los
// comodines de --file.
func Parse(args []string) (Config, error) {
	home, _ := os.UserHomeDir()
	c := Config{Puerto: 22}

	var servidor, destino string
	var patrones Lista

	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.StringVar(&servidor, "url", "", "servidor sftp://usuario@host:puerto[/carpeta] (obligatorio)")
	fs.Var(&patrones, "file", "archivo o comodín a enviar (se puede repetir)")
	fs.StringVar(&destino, "dest", "", "carpeta remota donde guardar; si no existe se usa la del sistema")
	fs.StringVar(&c.KeyFile, "key-file", filepath.Join(home, ".ssh", "id_ed25519"), "clave privada SSH")
	fs.StringVar(&c.KnownHosts, "known-hosts", filepath.Join(home, ".ssh", "known_hosts"), "host keys conocidas")
	fs.BoolVar(&c.DryRun, "dry-run", false, "valida sin cifrar ni conectarse")
	fs.BoolVar(&c.Estricto, "strict-naming", false, "aborta si un nombre no cumple la nomenclatura")
	fs.BoolVar(&c.SinVerificar, "insecure-host-key", false, "no verifica la host key (solo laboratorio)")
	fs.BoolVar(&c.SinColor, "no-color", false, "salida sin color")

	if err := fs.Parse(args); err != nil {
		return c, err
	}

	// Los secretos se leen del entorno: en la línea de comandos serían
	// visibles para cualquier usuario del sistema.
	c.Passphrase = os.Getenv("SFT_PASSPHRASE")
	c.Password = os.Getenv("SFT_SSH_PASSWORD")

	rutaURL, err := c.leerURL(servidor)
	if err != nil {
		return c, err
	}

	// --dest manda sobre la carpeta que venga en la URL.
	c.Destino = primero(destino, rutaURL)

	if len(patrones) == 0 {
		return c, errors.New("falta --file con el archivo o los archivos a enviar")
	}
	if c.Passphrase == "" && !c.DryRun {
		return c, errors.New(`falta la clave de cifrado. Defínala y vuelva a ejecutar:
    $env:SFT_PASSPHRASE = "la-clave-que-usted-elija"
  Debe ser la misma con la que la sede destino ejecute 'receive'`)
	}

	c.Archivos, err = Expandir(patrones)
	return c, err
}

// leerURL separa sftp://usuario@host:puerto/carpeta y devuelve la carpeta.
func (c *Config) leerURL(crudo string) (string, error) {
	if crudo == "" {
		return "", errors.New("falta --url (formato sftp://usuario@host:puerto)")
	}

	u, err := url.Parse(crudo)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("--url inválida: %s", crudo)
	}
	if u.Scheme != "sftp" && u.Scheme != "ssh" {
		return "", fmt.Errorf("esquema %q no soportado, use sftp://", u.Scheme)
	}
	if u.User == nil || u.User.Username() == "" {
		return "", errors.New("--url debe incluir el usuario: sftp://usuario@host")
	}

	c.Host = u.Hostname()
	c.Usuario = u.User.Username()

	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("puerto inválido: %s", p)
		}
		c.Puerto = n
	}

	return strings.TrimRight(u.Path, "/"), nil
}

// Expandir resuelve los comodines de --file y descarta carpetas y repetidos.
// Solo se procesan los archivos que el operador haya indicado.
func Expandir(patrones []string) ([]string, error) {
	vistos := map[string]bool{}
	var rutas []string

	for _, p := range patrones {
		encontrados, err := filepath.Glob(p)
		if err != nil || len(encontrados) == 0 {
			return nil, fmt.Errorf("ningún archivo coincide con %q", p)
		}
		for _, r := range encontrados {
			info, err := os.Stat(r)
			if err != nil || info.IsDir() || vistos[r] {
				continue
			}
			vistos[r] = true
			rutas = append(rutas, r)
		}
	}

	if len(rutas) == 0 {
		return nil, errors.New("no se encontró ningún archivo que enviar")
	}
	return rutas, nil
}

// CarpetasPorDefecto son las carpetas que se usan cuando la indicada con
// --dest no existe en el servidor. Estas sí se crean si hace falta.
func (c Config) CarpetasPorDefecto() []string {
	return []string{
		"/home/" + c.Usuario + "/sftnode",    // Linux
		"/tmp/sftnode",                       // Linux, siempre escribible
		"C:/Users/" + c.Usuario + "/sftnode", // Windows
	}
}

// Servidor devuelve "host:puerto", el formato que espera el cliente SSH.
func (c Config) Servidor() string { return fmt.Sprintf("%s:%d", c.Host, c.Puerto) }

func primero(valores ...string) string {
	for _, v := range valores {
		if v != "" {
			return v
		}
	}
	return ""
}
