# Diagrammes — sources PlantUML

Chaque `.puml` produit un `.svg` dans le répertoire parent, référencé
depuis `documentation_french/README.md`.

## Rendu

Depuis ce répertoire, avec `plantuml` sur le `PATH` :

```bash
plantuml -tsvg -o ../ *.puml
```

Le fichier `octo-theme.puml` est un `!include` partagé et n'est pas
rendu directement. C'est une copie de `docs/diagrams/octo-theme.puml`
gardée locale pour rendre ce répertoire auto-portant.
