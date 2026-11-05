# Registro dei problemi operativi Council — GTP 013

**Rilevazione:** 5 novembre 2026, ore 14:31 circa, Europe/Rome.
**Ambito:** uso reale di Council per la revisione e l'implementazione della feature GTP 013 con Codex, Claude Code, Antigravity/agy e Junie.
**Scopo:** conservare gli incidenti osservati, attribuirli al componente corretto e distinguere correzioni dimostrate, mitigazioni e problemi ancora aperti. Questo documento è una fotografia verificata; non è il cruscotto aggiornato dello sviluppo applicativo.

Il documento non avvia agenti, non modifica il workflow in corso e non autorizza nuove implementazioni. Non contiene credenziali, URL di controllo autenticati o trascrizioni integrali.

## Valutazione complessiva

Council è stato utilizzato realmente: esistono chiamate native/ACP, artefatti in worktree isolati, revisioni indipendenti e una revisione dell'autore dopo i finding. Non sono stati soltanto assegnati nomi a valutazioni del coordinatore.

Il coordinamento ha però richiesto molti interventi manuali. Timeout, incompatibilità tra istruzioni e capacità del trasporto, errori di preparazione del contesto e limiti dei CLI hanno rallentato il lavoro. Una parte consistente delle correzioni, dei collaudi e dell'integrazione è stata svolta direttamente dal coordinatore. Non è corretto attribuire tutto il risultato agli agenti delegati, né considerare conclusa la feature perché un workflow risulta `completed`.

### Contributi effettivamente documentati

| Partecipante | Lavoro osservato | Limite dell'evidenza |
|---|---|---|
| Claude Code | Autore dei test iniziali e dell'implementazione della fase 2; revisione successiva ai finding; ulteriori review in lettura | La prima scrittura è terminata per timeout; la fase 2 ha completato il ciclo degli agenti, che resta distinto dall'accettazione del coordinatore |
| Junie | Review di fixture e fase 2; individuazione del problema del marcatore di riconciliazione nella chiusura per inattività | Il suo output ACP non ha funzionato come voto JSON rigoroso; le review in prosa sono invece state utilizzabili |
| Codex nativo | Review del codice fornito nel contesto; individuazione della race tra acquisizione di una connessione e chiusura delle fixture | Alcuni comandi locali non erano eseguibili nel suo launcher; una precedente risposta `completed` dichiarava esplicitamente di non aver esaminato sorgenti |
| Antigravity/agy | Analisi e review, compresa la review finale delle fixture e una prova reale sul plugin | Un workflow ha ricevuto un aggiornamento di protocollo non valido; i successivi successi non ne dimostrano la risoluzione |
| Coordinatore Codex della chat principale | Preparazione dei task, gestione degli incidenti, correzioni al plugin e alle fixture, ulteriori finding applicativi, verifiche indipendenti, integrazione delle patch e commit locali | Il ruolo ha incluso implementazione diretta; non è stato soltanto un supervisore |

Non è disponibile una misura affidabile della percentuale di codice attribuibile a ciascuno. Il committer Git, da solo, non identifica chi abbia scritto ogni modifica.

## Stati e priorità

- **Risolto verificato:** correzione presente con evidenza pertinente.
- **Mitigato:** esiste una modalità operativa utilizzabile, ma la causa o la funzionalità originaria non è completamente risolta.
- **Aperto:** manca una diagnosi o una verifica sufficiente.
- **Da migliorare:** limite di coordinamento o tracciamento, non necessariamente un bug del plugin.

**P1** indica rischio di risultati non attendibili o di un passaggio di fase errato; **P2** indica soprattutto affidabilità operativa, manutenzione o latenza. Le priorità di questo registro non sono severity SonarQube.

## Problemi e azioni

### COU-01 — Runtime Node/SQLite iniziale incompatibile

- **Categoria / priorità:** ambiente, P1. **Stato:** mitigato tramite runtime dedicato.
- **Fatto:** il Node di sistema 24.14.1 esponeva SQLite 3.51.2, inferiore al requisito di sicurezza WAL dichiarato da Council. È stato usato Node 24.22.1 con SQLite 3.53.4, senza cambiare globalmente Node.
- **Impatto:** il semplice fatto che `node` fosse disponibile non dimostrava che Council potesse funzionare correttamente.
- **Azione:** verificare l'eseguibile effettivo e la versione SQLite prima dell'ammissione dei workflow. Anche npm deve essere eseguito con quel Node: la risoluzione del solo `PATH` può non bastare quando uno script usa un eseguibile adiacente.
- **Chiusura:** launcher e comandi di verifica devono risolvere stabilmente lo stesso runtime compatibile. La mitigazione locale non dimostra compatibilità con il Node di sistema.

### COU-02 — Output Junie incompatibile con il voto JSON rigoroso

- **Categoria / priorità:** interoperabilità e scelta del ruolo, P1. **Stato:** mitigato.
- **Fatto:** Junie ACP anteponeva messaggi di servizio/progresso al voto JSON. La stanza `a65667c0-cf22-4656-a6c8-a37dcc7ad18c` è stata cancellata dopo la verifica del problema.
- **Impatto:** una risposta leggibile e potenzialmente utile non rispettava il contratto macchina necessario al voto.
- **Correzione operativa:** Junie è stato usato come advisor/revisore in prosa; il parser rigoroso non è stato allentato e il voto non è stato ripulito silenziosamente.
- **Chiusura:** una futura qualifica del ruolo di votante deve dimostrare separazione corretta tra eventi di servizio e risposta finale, conservando gli errori reali. Le review in prosa riuscite non chiudono questo punto.

### COU-03 — Istruzioni READ non allineate ai tool effettivamente disponibili

- **Categoria / priorità:** plugin, P1. **Stato:** risolto verificato per la correzione descritta.
- **Fatto:** nella stanza `5279c59b-569d-48c9-9cda-43b4304edbbf`, Claude ha tentato Bash durante una verifica della toolchain, mentre READ consentiva soltanto `Read`, `Glob`, `Grep`. La stanza è fallita; non era un test backend fallito.
- **Correzione:** il plugin ora inserisce istruzioni dipendenti dal trasporto e dal permesso del singolo step. La dichiarazione CLI di Claude e il controllo del protocollo condividono la stessa allowlist. Anche un revisore READ in un workflow complessivamente WRITE riceve le istruzioni corrette; autore e revisione mantengono WRITE.
- **Codice:** [agent-permissions.ts](../src/agent-permissions.ts), [runner.ts](../src/runner.ts), [engine.ts](../src/engine.ts).
- **Evidenza:** commit locale del plugin `de35284`, con runtime generato; check/build e suite offline da 1.271 test, 1.262 passati, nove skip di piattaforma, zero fallimenti. Review indipendente `94b3fc43-fee0-46c2-9234-31f7265f1795`.
- **Limite:** la correzione non amplia i permessi e non costituisce una sandbox universale del sistema operativo.

### COU-04 — Launcher Codex incapace di eseguire alcuni comandi locali

- **Categoria / priorità:** ambiente/trasporto, P1. **Stato:** mitigato; causa non risolta.
- **Fatto:** alcuni comandi hanno restituito `registered Core launch requires an installed app`.
- **Impatto:** Codex non poteva verificare autonomamente toolchain o alcuni dati del checkout attraverso quei comandi. Questo non provava un difetto del progetto GTP.
- **Mitigazione:** il coordinatore ha eseguito i controlli; per una review successiva ha fornito integralmente i sorgenti nel contesto, senza richiedere comandi locali. Il workflow `828ce02a-52ea-473c-b144-c45b25732d9b` contiene una vera review Codex con finding.
- **Chiusura:** riprodurre e correggere il problema del launcher, poi verificare i comandi nel medesimo ambiente. Aumentare i permessi o dichiarare risolto il problema dopo una review testuale non sarebbe una verifica sufficiente.

### COU-05 — Restrizione del task incompatibile con gli strumenti del revisore

- **Categoria / priorità:** preparazione del task e interpretazione dell'esito, P1. **Stato:** mitigato.
- **Fatto:** nel workflow `5228b263-3f95-4094-ac9f-2d0ac1313dee`, il task vietava comandi shell anche a Codex, che dichiarava di non avere un'altra interfaccia consentita per leggere i file. Lo step risultava `completed`, ma il testo diceva esplicitamente che nessun sorgente era stato esaminato.
- **Impatto:** interpretare `completed` come approvazione della review avrebbe prodotto una falsa evidenza.
- **Azione:** preparare istruzioni per le capacità reali del partecipante. Se non sono disponibili letture locali consentite, fornire i sorgenti necessari. Registrare separatamente completamento della risposta, esecuzione della review ed esito della review.
- **Chiusura:** un esito di review deve indicare cosa è stato realmente esaminato; una risposta che dichiara l'impossibilità di farlo non vale come approvazione.

### COU-06 — Aggiornamento di protocollo Antigravity non valido

- **Categoria / priorità:** interoperabilità, P1. **Stato:** aperto, non riprodotto nelle prove successive.
- **Fatto:** lo step agy nel workflow `5228b263-3f95-4094-ac9f-2d0ac1313dee` è fallito con `Antigravity returned an invalid step update`.
- **Evidenza successiva:** la prova reale `79be8929-9bfc-4d21-8c45-55cd1c6eb228` e la review finale `828ce02a-52ea-473c-b144-c45b25732d9b` sono riuscite.
- **Azione:** conservare l'incidente e, se ricompare, acquisire la forma minima dell'evento e le versioni coinvolte, evitando contenuti sensibili. Distinguere un evento valido non supportato da un errore reale del provider.
- **Chiusura:** riproduzione, diagnosi, eventuale correzione e test del caso concreto. Un run successivo riuscito non basta; non accettare eventi malformati per trasformare il precedente fallimento in successo.

### COU-07 — Timeout della prima fase con artefatto incompleto

- **Categoria / priorità:** dimensionamento del task e recupero, P1. **Stato:** recupero completato; dimensionamento da migliorare.
- **Fatto:** l'autore del workflow `fbb264c4-0c16-476f-b7b0-32dfd0bc9449` ha raggiunto il limite di circa 15 minuti (`900486 ms`). Il workflow è rimasto `failed`; gli step di review e revisione previsti non erano stati eseguiti.
- **Impatto:** erano presenti test e fixture parziali; i test già eseguiti non coprivano automaticamente gli ultimi file scritti. L'esistenza della patch non dimostrava il completamento della fase.
- **Recupero:** conservazione dell'artefatto, nuovi task espliciti di review, correzioni e verifiche del coordinatore, poi accettazione e commit. Il run fallito non è stato rinominato come riuscito. Il timeout delle chiamate successive è stato portato a un'ora.
- **Azione:** dimensionare incarichi e budget sul lavoro effettivo, con incrementi coerenti e punti di consegna verificabili. Aumentare il timeout evita quella scadenza, ma non risolve da solo lentezza o task troppo ampi. Non rilanciare automaticamente scritture incomplete.

### COU-08 — Contesto della fase precedente passato al nuovo task

- **Categoria / priorità:** errore del coordinatore, P1. **Stato:** incidente chiuso; prevenzione da consolidare.
- **Fatto:** il primo avvio della fase 2 conteneva ancora il vincolo di lavorare soltanto sulla fase 1. Il workflow `9e1313e2-6901-4b52-9f9f-eba81144c5b3` è stato cancellato; l'artefatto esportato risultava `changed=false`.
- **Recupero:** nuovo incarico esplicito con contesto corretto, workflow `7b1bb9fb-6352-45cf-9960-2532bc1edf24`. Nessuna modifica di quell'avvio cancellato è stata integrata.
- **Azione:** prima di ogni avvio confrontare fase/task, contesto, commit base e permessi. Generare il riepilogo dal commit accettato corrente, evitando intestazioni ereditate da file della fase precedente.
- **Attribuzione:** non è un bug del parser o degli agenti: il coordinatore ha fornito input contraddittori.

### COU-09 — Identificativi delle sessioni e risultati parziali non sempre conservati

- **Categoria / priorità:** tracciamento del coordinatore, P1. **Stato:** mitigato.
- **Fatto:** in alcuni controlli è stato stampato soltanto `output`, perdendo il `session_id`; in un altro passaggio l'output non era più disponibile nel contesto della conversazione.
- **Impatto:** riprendere il processo o determinarne con certezza il completamento diventava più difficile. Output iniziale verde non significava suite completata.
- **Azione:** conservare risultato strutturato, identificativo, comando, ambito, log, codice di uscita e riepilogo finale. Per una ripresa, prima verificare il processo già avviato; ripetere un controllo solo quando serve, senza inventarne il precedente esito.
- **Limite:** i successivi run completi verificati non dimostrano retroattivamente il completamento di ogni run iniziale privo di evidenza finale.

### COU-10 — Codifica Windows non esplicita nelle modifiche documentali

- **Categoria / priorità:** errore negli strumenti del coordinatore, P2. **Stato:** risolto verificato per i file interessati.
- **Fatto:** l'uso della codifica predefinita Python su Windows ha introdotto byte non UTF-8 nelle nuove note della fase 1; una successiva lettura ha prodotto anche caratteri sostitutivi.
- **Correzione:** ricostruzione dei corpi originali dalla baseline verificata, riscrittura UTF-8 esplicita e scansione dei file modificati. Il follow-up applicativo `cc190ff` comprende la correzione. L'archivio storico è rimasto invariato.
- **Azione:** dichiarare `encoding='utf-8'` nelle letture/scritture e controllare decodifica rigorosa e caratteri sostitutivi prima dell'integrazione. Non copiare indiscriminatamente documenti da un worktree basato su una revisione precedente.

### COU-11 — Errori di invocazione CLI e preparazione dei file di richiesta

- **Categoria / priorità:** uso degli strumenti da parte del coordinatore, P2. **Stato:** episodi corretti; prevenzione da migliorare.
- **Fatti nella cronologia:** opzione CLI `--permission` non supportata, alias diagnostico non valido, interpolazione PowerShell errata del percorso del plugin e file JSON di richiesta inizialmente assente.
- **Impatto:** chiamate di coordinamento fallite o ripetute, rumore diagnostico e tempo perso. Non sono errori funzionali GTP, né prova che un modello abbia eseguito il lavoro richiesto.
- **Azione:** usare la sintassi documentata — lettura predefinita, `--write` quando autorizzato — e verificare schema, file e percorsi prima dell'esecuzione. Conservare l'errore originale quando si corregge l'invocazione. Non tutti questi episodi hanno un log separato ancora disponibile: parte dell'evidenza è nella cronologia della chat.

### COU-12 — Stato del piano meno preciso dello stato dei workflow

- **Categoria / priorità:** coordinamento e osservabilità, P2. **Stato:** da migliorare.
- **Fatto:** nella fotografia esaminata, `execution-plan.json` riportava la fase 2 come `authoring`, mentre il run corrispondente aveva già completato autore, review e revisione; erano inoltre presenti verifiche aggiuntive del coordinatore.
- **Impatto:** difficile capire a colpo d'occhio chi stia lavorando, quale verifica manchi e se il risultato sia integrato. `completed` del run non rende automaticamente completata la fase, ma `authoring` non descrive bene una verifica finale del coordinatore.
- **Azione:** distinguere almeno lavorazione dell'autore, review, revisione, verifica del coordinatore, integrazione e fase accettata con relativo commit. Collegare l'ultimo stato operativo alle evidenze, mantenendo la cronologia delle rettifiche.
- **Chiusura:** piano, report e stato della fase devono concordare sul prossimo passaggio necessario, senza confondere disponibilità di una patch con accettazione del codice.

### COU-13 — Latenza elevata e molto lavoro manuale del coordinatore

- **Categoria / priorità:** organizzazione e prestazioni, P2. **Stato:** da migliorare.
- **Evidenza:** il ciclo degli agenti della fase 2 ha richiesto circa 48 minuti e 4 secondi per l'autore, 4 minuti e 29 secondi per Junie e 9 minuti e 23 secondi per la revisione dell'autore: circa **61 minuti e 56 secondi nei soli tre step**, oltre alle verifiche e all'integrazione del coordinatore. Questi tempi includono attività dei tool; non misurano soltanto inferenza, costo o produttività del modello.
- **Impatto:** i quattro agenti non stavano implementando quattro parti in parallelo. Il coordinatore ha dovuto recuperare artefatti, correggere codice, predisporre contesti, verificare e integrare manualmente.
- **Azione:** usare incarichi più circoscritti, parallelizzare solo parti realmente indipendenti e rendere esplicito ciò che resta al coordinatore. Evitare audit duplicati senza nuovo oggetto di verifica. Conservare i controlli necessari dopo modifiche e finding; le ripetizioni motivate non sono automaticamente spreco.
- **Limite di progetto:** Council non mantiene autonomamente attivo il modello della chat coordinatrice. L'integrazione manuale è anche una scelta di sicurezza del workflow: non va eliminata introducendo merge automatici incompatibili con il mandato.

### COU-14 — Preparazione manuale dei metadati ignorati nei worktree

- **Categoria / priorità:** integrazione locale, P2. **Stato:** mitigato manualmente.
- **Fatto:** i worktree puliti non portano automaticamente i metadati locali ignorati di Spec Kit e delle skill. Il coordinatore ha predisposto esplicitamente i file non segreti necessari.
- **Impatto:** il checkout del commit giusto non dimostra che tutti gli strumenti locali richiesti dal task siano disponibili.
- **Azione:** dichiarare prerequisiti e, se necessario, un elenco esplicito dei soli metadati da predisporre. Non copiare tutti i file ignorati: `.env`, credenziali e configurazioni personali devono restare esclusi.
- **Attribuzione:** è il comportamento normale di un worktree Git pulito; l'esigenza di preparazione non dimostra da sola un difetto di Council.

### COU-15 — Review Junie interrotta da una richiesta di approvazione ACP

- **Categoria / priorità:** interoperabilità e capacità del revisore, P1. **Stato:** aperto; artefatto recuperato senza ripetere le scritture.
- **Fatto successivo alla fotografia iniziale:** il workflow di fase3 `f159fc48-ba39-4ebe-b86c-978cfd2c1f83` ha completato l'autore Claude (`3226094 ms`, circa53 minuti e46 secondi), poi è fallito durante la review Junie (`135703 ms`) con `ACP tool permission was denied by read mode; the task is incomplete.` Il task indicava già letture circoscritte, niente terminale/MCP o nuove approvazioni.
- **Impatto:** l'autore aveva prodotto codice e verifiche; non esiste una review Junie completata né la successiva revisione dell'autore di quel workflow. Il solo artefatto non consentiva il passaggio di fase.
- **Recupero effettivo:** conservati run fallito e worktree; il coordinatore ha esaminato i sorgenti, corretto i test e ripetuto535 test mirati con esito positivo. Una review Codex separata, `d9ed111e-7021-4165-a900-5945ea897384`, ha ricevuto i file completi nel contesto e ha prodotto un finding applicativo concreto. Nessuna scrittura dell'autore è stata rilanciata automaticamente e READ non è stato trasformato in WRITE.
- **Chiusura:** identificare lo specifico strumento che ha richiesto approvazione e qualificare un percorso di review conforme alle capacità ACP. Il recupero con un altro percorso non dimostra che Junie possa completare quella stessa review. Il criterio fail-closed del plugin non è di per sé un difetto da aggirare.

### COU-16 — Diagnostica insufficiente sulla richiesta di permesso negata

- **Categoria / priorità:** osservabilità del plugin, P2. **Stato:** aperto, nessuna modifica al runtime effettuata per questo punto.
- **Fatto:** il risultato conservato di COU-15 indica soltanto che una richiesta `session/request_permission` è stata negata in READ. La gestione in `src/acp.ts` restituisce quel messaggio generico senza identificare nel risultato il tipo sicuro dell'operazione richiesta. Non è quindi possibile attribuire con certezza l'incidente a terminale, modifica, ricerca o altra azione leggendo il risultato finale.
- **Impatto:** dopo oltre50 minuti di lavoro dell'autore, la diagnosi e la preparazione del recupero richiedono altre ispezioni; non è giustificato inventare il tool che Junie avrebbe tentato.
- **Azione proposta:** conservare metadati minimi e non sensibili della richiesta, per esempio il `kind` ACP validato, evitando comandi, input, percorsi riservati o credenziali. Verificare con test che diagnostica ed esito restino accurati e che READ continui a negare nuove approvazioni.
- **Chiusura:** riproduzione con evidenza sicura del tipo di operazione e test pertinenti; nessun successo successivo chiude automaticamente questa lacuna.

### Aggiornamento COU-13 e sospensione dell'orchestrazione

La fase3 aggiunge un secondo esempio di ciclo lungo: circa53 minuti e46 secondi per l'autore, seguito da circa2 minuti e16 secondi di review poi fallita. Sono tempi degli step, comprensivi di tool; non una misura di sola inferenza. Il coordinatore ha nuovamente dovuto recuperare, correggere, verificare e integrare manualmente. Il finding sul binding degli engine è stato riprodotto con48 casi rossi prima della correzione: una review utile non equivale ancora a una fase accettata.

Il 5 novembre2026 il committente ha chiesto esplicitamente di **non usare più Council** e di proseguire da soli. Non vengono avviati altri workflow, advisor o agenti tramite il plugin. Il dashboard locale gestito dal coordinatore è stato arrestato dopo verifica di PID, eseguibile, comando e data di creazione; gli artefatti e gli esiti precedenti restano conservati. L'aggiornamento di questo registro non dichiara risolti COU-15/16 e non riavvia l'orchestrazione.

## Risultati che non vanno confusi con problemi di Council

Le review hanno individuato problemi reali nelle fixture e nel codice applicativo: identificativi conservati dopo rollback, collisioni tra fixture, ACK positivo durante shutdown, race acquisizione/chiusura connessioni e perdita del marcatore dopo un timeout. Questo dimostra utilità delle review; quei difetti non sono bug del plugin.

Anche i blocchi infrastrutturali vanno separati: il recupero dell'immagine MinIO predefinita ha restituito HTTP404; i test sono poi stati eseguiti con l'override compatibile già previsto. Le parti del flow IMP ancora da completare e la qualificazione sul target NiFi restano questioni di integrazione applicativa. Nessuna suite offline di Council o fixture locale può certificarle.

Le 12 asserzioni rosse osservate in un passaggio della suite di concorrenza della fase 2 facevano parte di un **controllo negativo deliberato**: l'autore aveva temporaneamente rimosso protezioni per dimostrare che i test rilevassero la regressione. Il totale dichiarato del controllo negativo era di 14 test falliti su più suite; i sorgenti sono stati ripristinati e i controlli ripetuti. Non registrare quell'episodio come guasto spontaneo del plugin o del codice finale.

La review Codex di fase3 ha rilevato che il binding enum permissivo di Boot accettava alias come `N-I-F-I`. È un problema applicativo/configurativo, non del plugin. La prima correzione del coordinatore con un converter che lanciava un'eccezione non bastava: il binder tentava successivamente la conversione permissiva. Il primo riesame mirato è rimasto fallito (49 asserzioni su584); dopo l'adeguamento del converter e della validazione già esistente, i396 casi di binding sono passati. Questo non sostituisce il successivo riesame complessivo della fase.

Su richiesta esplicita è stata inoltre creata in `utils` una fixture HTTP/DB NiFi locale, commit `c2739b3`:17 test passati, inclusi6 PostgreSQL sullo schema reale V1–V12 in un container effimero. La review agy `06551a66-2266-42dd-974b-06f5697056cc` è stata completata senza finding, con verifiche e revisione del coordinatore distinte. La fixture non genera PdV/ZIP e non certifica IMP. Il comando generale delle utility è rimasto fallito sul registro di snapshot applicativi non aggiornato: nessuna fixture o suite del plugin può convertirlo automaticamente in evidenza positiva.

## Evidenze e confini di verifica

Gli identificativi seguenti servono a rintracciare i risultati conservati, non rappresentano certificazioni intercambiabili.

| Evidenza | Esito effettivo | Cosa prova |
|---|---|---|
| `906060f7-5145-4a98-899f-e8156fad0bb3` | Chiamate reali dei quattro agenti completate | Partecipazione reale iniziale; non completamento dello sviluppo |
| `fbb264c4-0c16-476f-b7b0-32dfd0bc9449` | Fallito per timeout dell'autore | Artefatto parziale conservato; review/revisione originarie non eseguite |
| `5228b263-3f95-4094-ac9f-2d0ac1313dee` | Workflow fallito; agy fallito, Codex dichiarava review non eseguita, Junie aveva prodotto una review | Necessità di leggere contenuto e risultato di ogni step |
| `81cea542-a432-4f68-9e97-bed40a9b7ae0` | Review Claude/Junie completate | Finding reali sulle fixture, da correggere e riverificare |
| `828ce02a-52ea-473c-b144-c45b25732d9b` | Review native Codex/agy completate | agy senza finding; Codex ha rilevato una race residua, poi corretta dal coordinatore |
| `9e1313e2-6901-4b52-9f9f-eba81144c5b3` | Cancellato; artefatto senza modifiche | Recupero dell'errore di contesto, non fase eseguita |
| `7b1bb9fb-6352-45cf-9960-2532bc1edf24` | Autore Claude, review Junie e revisione Claude completati | Produzione reale dell'artefatto di fase 2 e correzione del finding; accettazione successiva separata |
| Suite offline del plugin | 1.271 test: 1.262 passati, nove skip di piattaforma, zero fallimenti | Regressioni del plugin verificate; non qualifica universale dei provider nativi |
| Ultimo log backend del coordinatore letto per questo registro | 2.477 test, zero fallimenti/errori, quattro skip live; exit 0 | Verifica backend locale aggiuntiva presente al momento della rilevazione; non collaudo reale IMP/NiFi o completamento delle otto fasi |

Il pacchetto di evidenze locale esaminato si trova in `C:\devel\smc\acs\gtp\.agent_workspace\council013`. I riferimenti principali sono `SESSION.md`, `execution-plan.json`, `phase-01-result.json`, `phase-01-partial-review.json`, `phase-01-fixed-review.json`, `phase-01-final-source-review.json`, `phase-02-stale-context-result.json`, `phase-02-result.json`, `plugin-independent-review.json`, `agy-probe.json`, `plugin-final-tests.log` e `phase-02-host-full.log`, con i relativi codici di uscita dove disponibili. Sono evidenze locali esterne al plugin; non sono prerequisiti del suo runtime.

I contatori di test non vanno sommati tra esecuzioni successive per rappresentare una copertura maggiore. Una verifica dell'autore, una verifica del coordinatore e un collaudo sul target sono evidenze distinte. Restano inoltre distinti: patch generata, codice revisionato, codice verificato, commit locale, push e deploy.

## Come aggiornare il registro

Per ogni nuovo incidente registrare ID, categoria, responsabile della correzione, evento riproducibile, impatto, evidenza, stato e criterio di chiusura. Correggere espressamente le interpretazioni precedenti quando emergono nuove prove. Un workaround può rendere utilizzabile il lavoro senza chiudere la causa originaria.

Le priorità operative sono chiudere o caratterizzare COU-02/04/06, prevenire nuovi errori COU-08/09/11 e rendere preciso il tracciamento COU-12/13. Ogni eventuale modifica al plugin resta un intervento separato, con controlli pertinenti: questo registro documentale non la esegue né la dichiara completata.
