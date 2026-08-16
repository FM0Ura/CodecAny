package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// embeddedWebdist embute o build do frontend React/Vite (gerado em /web,
// saída apontada para cmd/server/webdist/ — ver docs/propostas_painel_controle.md,
// seção 1). "all:" inclui também arquivos começando com "_"/"." se algum
// dia existirem (padrão de build de bundlers modernos).
//
//go:embed all:webdist
var embeddedWebdist embed.FS

// newAssetHandler retorna o http.Handler que serve o SPA embutido, com
// fallback de SPA: qualquer caminho sem correspondência exata no FS
// embutido cai para index.html — necessário para o roteamento client-side
// (React Router) funcionar depois que o frontend real for buildado.
func newAssetHandler() (http.Handler, error) {
	sub, err := fs.Sub(embeddedWebdist, "webdist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServerFS(sub)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "."
		}
		if _, statErr := fs.Stat(sub, p); statErr != nil {
			// Caminho sem correspondência no FS embutido: provavelmente uma
			// rota client-side do SPA (React Router) — reescreve para "/" e
			// deixa o fileServer servir index.html em vez de 404.
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}
