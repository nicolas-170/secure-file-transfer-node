// Package envio abre el canal seguro SSH y transfiere los paquetes por SFTP.
package envio

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/nicolas/sftnode/internal/config"
	"github.com/nicolas/sftnode/internal/paquete"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Cliente mantiene una única conexión SSH reutilizada por todos los archivos.
type Cliente struct {
	ssh  *ssh.Client
	sftp *sftp.Client
}

// Conectar establece el canal seguro con la sede destino.
func Conectar(c config.Config) (*Cliente, error) {
	auth, err := credenciales(c)
	if err != nil {
		return nil, err
	}
	hostKey, err := verificador(c)
	if err != nil {
		return nil, err
	}

	conexion, err := ssh.Dial("tcp", c.Servidor(), &ssh.ClientConfig{
		User:            c.Usuario,
		Auth:            auth,
		HostKeyCallback: hostKey,
		Timeout:         30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a %s: %w", c.Servidor(), err)
	}

	// SFTP es un subsistema que viaja dentro del canal SSH ya cifrado.
	// Las escrituras concurrentes aceleran mucho los archivos grandes,
	// como el propio binario que se copia a la sede.
	s, err := sftp.NewClient(conexion, sftp.UseConcurrentWrites(true))
	if err != nil {
		conexion.Close()
		return nil, fmt.Errorf("no se pudo abrir la sesión SFTP: %w", err)
	}

	return &Cliente{ssh: conexion, sftp: s}, nil
}

// credenciales elige contraseña o clave privada, según lo disponible.
func credenciales(c config.Config) ([]ssh.AuthMethod, error) {
	if c.Password != "" {
		return []ssh.AuthMethod{ssh.Password(c.Password)}, nil
	}

	pem, err := os.ReadFile(c.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("no hay contraseña (SFT_SSH_PASSWORD) ni clave privada en %s", c.KeyFile)
	}
	clave, err := ssh.ParsePrivateKey(pem)
	if err != nil {
		return nil, fmt.Errorf("clave privada inválida o protegida con passphrase: %w", err)
	}
	return []ssh.AuthMethod{ssh.PublicKeys(clave)}, nil
}

// verificador comprueba que el servidor sea el esperado comparándolo con
// known_hosts. Sin esto, un atacante podría suplantar la sede destino.
func verificador(c config.Config) (ssh.HostKeyCallback, error) {
	if c.SinVerificar {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	cb, err := knownhosts.New(c.KnownHosts)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer %s; conéctese una vez con ssh para registrar la sede", c.KnownHosts)
	}
	return cb, nil
}

// ElegirCarpeta devuelve la carpeta donde guardar los paquetes.
//
// La que pide el operador con --dest debe existir ya en el servidor: el
// programa no crea carpetas que no le pertenecen. Si no existe, se recurre a
// las carpetas por defecto, y esas sí se crean.
func (cl *Cliente) ElegirCarpeta(pedida string, porDefecto []string) (string, error) {
	if pedida != "" && cl.existe(pedida) {
		return pedida, nil
	}
	for _, ruta := range porDefecto {
		if err := cl.sftp.MkdirAll(ruta); err == nil {
			return ruta, nil
		}
	}
	return "", fmt.Errorf("ninguna carpeta destino se pudo usar: %v", porDefecto)
}

// existe indica si la ruta remota es una carpeta ya creada.
func (cl *Cliente) existe(ruta string) bool {
	info, err := cl.sftp.Stat(ruta)
	return err == nil && info.IsDir()
}

// Enviar sube el paquete a la carpeta indicada. Se escribe con extensión
// .part y se renombra al final, para que la sede destino nunca vea un
// archivo a medio transferir.
func (cl *Cliente) Enviar(datos []byte, nombre, carpeta string) error {
	destino := path.Join(carpeta, nombre)
	temporal := destino + ".part"

	f, err := cl.sftp.Create(temporal)
	if err != nil {
		return err
	}
	if _, err := f.Write(datos); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	cl.sftp.Remove(destino) // por si quedó un envío anterior
	return cl.sftp.Rename(temporal, destino)
}

// EnviarPrograma copia el binario a la carpeta destino y lo deja ejecutable,
// reemplazando el que hubiera de un envío anterior.
func (cl *Cliente) EnviarPrograma(ruta, carpeta string) (string, error) {
	datos, err := os.ReadFile(ruta)
	if err != nil {
		return "", err
	}

	nombre := filepath.Base(ruta)
	if err := cl.Enviar(datos, nombre, carpeta); err != nil {
		return "", err
	}

	destino := path.Join(carpeta, nombre)
	if err := cl.sftp.Chmod(destino, 0o755); err != nil {
		return "", fmt.Errorf("no se pudo dar permiso de ejecución: %w", err)
	}
	return destino, nil
}

// Verificar relee el paquete ya guardado y comprueba que su hash coincide
// con el del archivo que se envió.
func (cl *Cliente) Verificar(nombre, carpeta, hashEsperado string) error {
	f, err := cl.sftp.Open(path.Join(carpeta, nombre))
	if err != nil {
		return err
	}
	defer f.Close()

	datos, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	if h := paquete.Hash(datos); h != hashEsperado {
		return fmt.Errorf("el paquete guardado no coincide (%s)", h[:12])
	}
	return nil
}

// Cerrar libera la sesión SFTP y la conexión SSH.
func (cl *Cliente) Cerrar() {
	cl.sftp.Close()
	cl.ssh.Close()
}
