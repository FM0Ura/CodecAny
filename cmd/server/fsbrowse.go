package main

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

// fsEntry é uma subpasta listada por GET /api/fs/browse.
type fsEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// fsBrowseResponse é o shape de GET /api/fs/browse?path=.
type fsBrowseResponse struct {
	Path    string    `json:"path"`
	Parent  string    `json:"parent"`
	Entries []fsEntry `json:"entries"`
}

// handleFsBrowse atende GET /api/fs/browse?path= — navegador de diretórios
// usado pelo modal de "Adicionar diretório" da UI (decisão de produto já
// fechada: preferido a um campo de texto puro — ver
// docs/propostas_painel_controle.md, seção 3/4). Implementado inteiramente
// aqui em cmd/server (os.ReadDir + filepath.Dir), sem tocar pkg/core:
// coerente com o modelo de confiança já assumido pelo servidor (sem auth,
// bind local) — quem acessa a API já pode aprovar/rejeitar jobs e adicionar
// diretórios monitorados, então navegar o filesystem não é uma classe de
// risco nova.
//
// Só diretórios aparecem em Entries (arquivos são filtrados), ordenados
// alfabeticamente por nome. path="" (default) retorna a raiz ("/"). Erros
// de permissão/inexistência respondem 400 (entrada do usuário), não 500.
func (a *App) handleFsBrowse(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}
	path = filepath.Clean(path)

	fi, err := os.Stat(path)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "diretório inacessível: "+err.Error())
		return
	}
	if !fi.IsDir() {
		writeJSONError(w, http.StatusBadRequest, "path não é um diretório")
		return
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "falha ao ler diretório: "+err.Error())
		return
	}

	out := make([]fsEntry, 0, len(entries))
	for _, e := range entries {
		full := filepath.Join(path, e.Name())
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			// os.DirEntry.IsDir() não segue links simbólicos — resolve
			// explicitamente para incluir symlinks que apontam para
			// diretórios (comum em bibliotecas de mídia montadas via link).
			info, statErr := os.Stat(full)
			isDir = statErr == nil && info.IsDir()
		}
		if !isDir {
			continue
		}
		out = append(out, fsEntry{Name: e.Name(), Path: full})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	parent := filepath.Dir(path)
	if parent == path {
		// Já está na raiz do filesystem — sem "voltar".
		parent = ""
	}

	writeJSON(w, http.StatusOK, fsBrowseResponse{Path: path, Parent: parent, Entries: out})
}
