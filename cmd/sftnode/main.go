// Comando sftnode: punto de entrada único del binario.
// Toda la lógica vive en internal/; aquí solo se delega y se propaga el
// código de salida al sistema operativo.
package main

import (
	"os"

	"github.com/nicolas/sftnode/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
