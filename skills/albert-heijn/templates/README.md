# Optional templates

`PROFILE.template.md`, one directory up, is the one that matters. It is what the skill reads.

These files are from the original file-based design, where the agent kept its state in JSON next to
the skill. Nothing in the Go code reads them, and the skill does not require them. They are here
because some people prefer structured state to prose, and because they are a reasonable starting
point if you want to script against your own list.

| File | What it was for |
|---|---|
| `config.template.json` | Meals per week, household size, cooking time, dislikes, allergies |
| `weekly-basics.template.json` | The staples that go on the list every week |
| `meal-history.template.json` | Which proposals were accepted or rejected, and why |
| `product-cache.template.json` | Product ids already looked up, to save a search |
| `taste-profile.template.md` | Free-text notes on taste, superseded by `PROFILE.md` |

Copy the ones you want, drop the `.template` from the name, and fill them in. Filled-in copies are
gitignored, which is deliberate: they describe a specific household.
