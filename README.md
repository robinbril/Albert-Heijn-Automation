# Albert Heijn Automation

Een agent die je boodschappen doet en je koopgedrag terugleest. Praat met de mobiele API van de Appie-app, via een CLI en een MCP-server.

> Onofficieel. Geen enkele band met Albert Heijn of Ahold Delhaize. De API is niet gedocumenteerd en kan zonder aankondiging veranderen. Eigen risico.

Gebouwd op [appie-go](https://github.com/gwillem/appie-go) van [@gwillem](https://github.com/gwillem).

## Installeren

Go 1.24 of nieuwer.

```bash
go build -o bin/ ./cmd/...
./bin/appie-cli login
```

Login opent je browser. De tokens komen in je user-config (`%AppData%\appie\tokens.json` op Windows, `~/.config/appie/tokens.json` elders) met mode 0600, en verversen daarna zichzelf. Stand van zaken: `appie-cli status`.

## Aan Claude hangen

```bash
claude mcp add ah -- /absoluut/pad/naar/bin/appie-mcp
```

35 tools. Mag hij alleen kijken, zet er `--env AH_READONLY=1` bij.

## Wat het doet

**Boodschappen.** `bonus` geeft de hele folder, `bonus-matches` filtert hem op wat je toch al koopt. `recipes` zoekt Allerhande met een limiet op werktijd, waarbij oven- en wachttijd niet meetellen, want daar sta je niet bij. `plan` voegt de ingrediënten van meerdere recepten samen zodat je uien één keer koopt. Je lijst en je winkelmand kun je lezen, aanvullen, aanpassen en leeghalen.

**Koopgedrag.** `purchases` zet online bestellingen en kassabonnen in één tijdlijn. `patterns` haalt daar je boodschapritme, je vaste dag en je vaste producten uit. `health` leest je mandje terug als eetpatroon: voedselgroepen, Nutri-Score-verdeling, en de D- en E-producten die je blijft herhalen. `swap` zoekt daar een beter scorend alternatief bij uit dezelfde categorie, met het prijsverschil. Beide beschrijven een boodschappenmandje en zijn geen medisch advies.

`appie-cli` zonder argumenten geeft de volledige lijst.

## Schrijfacties

Twee onafhankelijke sloten. Elke MCP-tool die iets verandert weigert zonder `confirm=true`, en die check zit in de registratie in plaats van in de handlers, dus een tool die er later bij komt kan hem niet vergeten. `AH_READONLY=1` blokkeert mutaties in de HTTP-transport, onder elke callsite: GraphQL-bodies worden op een `mutation`-operatie gecontroleerd en wat niet te parsen is telt als write.

Writes worden nooit geretried. Een dubbel toegevoegd product is erger dan één fout die je ziet.

Je naam, mailadres, telefoonnummer, adres en kaartnummers blijven uit `member` tenzij je `-pii` meegeeft. De segmenten die AH zelf van je maakt krijg je wel.

## Wat ik onderweg tegenkwam

- `previouslyBought` op `productSearch` staat nergens gedocumenteerd en geeft alles terug wat je ooit gekocht hebt.
- `customerProfileProperties` laat AH's eigen profilering zien: levensfase, foodprofiel, prijssegment, vaste boodschappendag.
- De lijst in de app is `shoppinglist/v2`, de favorietenlijsten zijn `lists/v3`. Twee losse collecties, wat je in de één zet verschijnt niet in de ander.
- Een regel verwijderen gaat met `quantity: 0`. `strikeThrough` streept hem alleen door en laat hem staan.
- Kassabonnen bevatten geen product-id's, alleen omschrijvingen. Van 92 producten kon ik er 2 terugvinden, dus `health` draait op je koopgeschiedenis.
- Eén kapotte bonuscategorie liet `appie-go` de hele folder weggooien. Hier gaat hij door en rapporteert wat oversloeg.
- Betalen en een bezorgslot kiezen kan niet. Daar is geen publieke API voor, dat blijft de app.
- Introspectie op de GraphQL staat uit en het Allerhande-schema schuift. Bij `GRAPHQL_VALIDATION_FAILED` draai je losse queries met `appie-cli graphql` en lees je de nieuwe vorm uit de validatiefouten.

## Development

```bash
go test ./...
go vet ./...
```

`internal/ah` is de client, de transport-guard en alles wat appie-go niet dekt. `cmd/appie-cli` en `cmd/appie-mcp` zijn dunne lagen daarboven.

AGPL-3.0
