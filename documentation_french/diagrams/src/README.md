# Diagrammes — sources PlantUML

Chaque `.puml` produit un `.svg` dans le répertoire parent, référencé
depuis `documentation_french/README.md`.

## Rendu

Depuis ce répertoire, avec `plantuml` sur le `PATH` :

```bash
plantuml -tsvg -o ../ *.puml
```

## Deux familles de diagrammes

- **Diagrammes de séquence** (`seq-*.puml`, `deux-couches-*.puml`,
  `anatomie-skill.puml`) — utilisent `!include octo-theme.puml` pour
  garder la charte visuelle du projet (bleu marine sur fond blanc).
  Le fichier `octo-theme.puml` est un `!include` partagé, pas rendu
  directement ; c'est une copie de `docs/diagrams/octo-theme.puml`
  pour rendre ce répertoire auto-portant.
- **Diagrammes d'architecture C4** (`c4-*.puml`) — utilisent la
  bibliothèque standard livrée avec PlantUML :
  `!include <C4/C4_Context>`, `<C4/C4_Container>`, `<C4/C4_Deployment>`.
  Aucune dépendance réseau. Le style visuel bleu/gris est la
  convention C4 de Simon Brown ; il diffère volontairement du thème
  Octo pour que les lecteurs familiers du modèle le reconnaissent
  immédiatement.
