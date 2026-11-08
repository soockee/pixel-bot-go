---
title: "Tune detection speed and accuracy"
summary: "Which settings make the search faster or more precise, and in which order to change them."
order: 5
---

| Symptom             | Adjust                                |
| ------------------- | ------------------------------------- |
| False positives     | Raise `Threshold`, enable `Refine`    |
| Slow detection      | Narrow the scale range                |
| Miss different size | Expand `MinScale..MaxScale`           |
| Memory spike        | Reduce scale count / reuse buffers    |
