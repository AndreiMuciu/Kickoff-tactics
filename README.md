# Kickoff Tactics

Auto-battler de fotbal. Ești antrenor: cumperi jucători, îi așezi pe teren, ei joacă singuri,
iar la pauză dai ordinul pentru repriza a doua. Demo local, un jucător contra echipe conduse de server.

Designul și deciziile sunt în [DESIGN.md](DESIGN.md).

## Pornire

Cerințe: Go și Node (testat cu Go 1.27, Node 26). MongoDB este opțional.

```bash
./run.sh            # backend pe 127.0.0.1:8080, frontend pe http://localhost:5173
```

Sau separat:

```bash
cd backend && go run ./cmd/server
cd frontend && npm ci && npm run dev
```

Fără `MONGODB_URI`, starea stă în memorie și se pierde la repornire. Cu MongoDB:

```bash
docker run -d --name footy-mongo -p 127.0.0.1:27017:27017 mongo:8
cp .env.example .env    # setează MONGODB_URI=mongodb://127.0.0.1:27017
./run.sh
```

Pentru MongoDB Atlas pui în `.env` URI-ul `mongodb+srv://...` al clusterului. Codul e același;
calea Atlas nu a fost încă rulată pe un cluster real.

## Structură

```
backend/
  cmd/server/          pornire, configurare din variabile de mediu
  internal/game/       regulile: date, economie, magazin, simularea meciului (fără HTTP)
  internal/store/      persistență: memorie sau MongoDB
  internal/api/        HTTP: sesiune, limitare, validare
frontend/
  src/api.ts           clientul HTTP și tipurile
  src/App.tsx          ecranul de management
  src/MatchView.tsx    redarea meciului pe canvas
```

## Cine decide ce

Serverul decide tot. Clientul trimite doar intenții (`buy`, `sell`, `move`, `reroll`, `levelup`,
start meci, ordinul de la pauză) și desenează ce primește înapoi: cadre de poziții și evenimente.
Bugetul, magazinul, zarurile și rezultatul nu există în browser decât ca afișaj.

## Securitate

| Risc | Măsură |
| --- | --- |
| Client modificat (buget, jucători, rezultat) | Toată starea și toate regulile sunt pe server; răspunsul către client e o proiecție (`RunView`) fără seed și fără starea generatorului |
| Ghicirea sau alegerea zarurilor | Seed din `crypto/rand`, ținut secret până la final |
| Server care „aranjează” meciul după ordinul tău | Commit-reveal: SHA-256 al seed-ului e afișat înainte de alegere, seed-ul după meci; repriza întâi e prefix exact al meciului complet (test) |
| Furt de sesiune prin XSS | Token de 256 de biți în cookie `HttpOnly`, `SameSite=Strict`, `Path=/api`; în bază se ține doar hash-ul |
| Parole | Argon2id (19 MiB, 2 treceri) cu sare aleatoare; maximum 4 hash-uri simultan; parola nu e logată și nu iese din server |
| Ghicirea parolelor | Limită per IP și per cont; răspuns și timp identice pentru utilizator inexistent și parolă greșită |
| Fixarea sesiunii | Token nou la fiecare autentificare; logout șterge sesiunea din bază |
| Acces la conversațiile altora | Chat și amicale doar între prieteni acceptați, verificat pe server la fiecare cerere |
| Conținut injectat în chat | Text simplu, maximum 500 de caractere, fără caractere de control; clientul îl afișează ca text |
| Privit înainte într-o serie live | Serverul trimite doar cadrele deja „întâmplate” plus 3 s; repriza a doua nu se calculează înainte de finalul pauzei; ordinul adversarului nu e trimis niciodată |
| CSRF | Verificare strictă de `Origin` pe orice cerere care schimbă stare, plus `SameSite=Strict` |
| Cheltuire dublă prin cereri paralele | Lacăt per sesiune și versiune optimistă la salvare (12 `reroll` simultane cu buget 3: unul singur trece) |
| Injecție în interogări Mongo | Filtre construite doar din valori de server cu tip fix; corpul JSON e decodat în structuri stricte |
| Corpuri mari, câmpuri ascunse | Limită 2 KB, `DisallowUnknownFields`, `Content-Type` obligatoriu, fără date după obiect |
| Abuz, forță brută | Limitare per IP: 15 cereri/s; sesiuni noi 10/minut |
| Conexiuni lente | Timeout-uri pe citire, scriere, antete; timeout de 5 s per cerere |
| Scurgeri de detalii interne | Erorile interne sunt doar în log; clientul primește mesaje fixe |
| Expunere accidentală | Ascultă pe `127.0.0.1`; secretele vin doar din mediu, `.env` e ignorat de git |

Antete pe răspunsurile API: `Content-Security-Policy: default-src 'none'`, `X-Frame-Options: DENY`,
`X-Content-Type-Options: nosniff`, `Cache-Control: no-store`, `Referrer-Policy: no-referrer`.

De făcut înainte de orice expunere publică: HTTPS cu `COOKIE_SECURE=true`, CSP și pentru paginile
frontend-ului, IP real din proxy de încredere pentru limitare, resetare de parolă și verificare de email.

## Teste

```bash
cd backend && go test ./...
```

Acoperă: determinism, repriza întâi ca prefix, echilibru (echipa mai bună câștigă 60-90%, goluri pe
meci în interval), simetria terenului, validarea intențiilor, unirea jucătorilor, commit-reveal.

## Poze și nume reale

Jucătorii au nume reale. Pozele se încarcă în browser din API-ul public Wikipedia, doar imagini cu
licență liberă (Wikimedia Commons), într-o singură cerere pe sesiune; fără poză liberă apar inițialele.
Browserul jucătorului contactează deci `en.wikipedia.org` și `*.wikimedia.org`.
Pentru o lansare publică e nevoie de licențe pentru nume și imagine; vezi DESIGN.md.

## Cunoscut

- `npm audit` raportează `picomatch` (ReDoS) adus de Vite. E unealtă de build, nu ajunge în browser.
- Există conturi, pagină de start, prieteni, chat și serie live între doi prieteni (sezon nou de la zero pentru amândoi, stare ținută în baza de date, sincronizare pe ceasul serverului). Lipsesc WebSocket-ul (totul e prin interogare periodică), resetarea parolei, cartonașele și itemele. Vezi DESIGN.md.
- `dev-test-users.txt` (ignorat de git) ține conturile de test locale create la verificare.
