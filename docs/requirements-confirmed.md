# Bestaetigte Anforderungen

## Funktionaler Umfang

- Alle relevanten OpenAI-kompatiblen Endpunkte sollen unterstuetzt werden (inkrementell).
- Alle verfuegbaren Modelle der Worker sollen automatisch erkannt und angeboten werden.
- Netzwerkzugriff nur intern.
- Admin-Authentifizierung ueber OIDC.
- Multi-Tenancy ist erforderlich.
- Quotas und Rate-Limits sind erforderlich.
- Load-Balancing erfolgt nach Health, Latenz und Kapazitaet.
- Bei Fehlern soll transparent auf andere Worker retried werden.
- Standard-Logging nur Metadaten; pro Token umschaltbarer Debug-Modus fuer volles Logging.
- Admin-UI ist erforderlich.
- High-Availability mit mehreren Proxy-Instanzen ist erforderlich.
- Echtzeit-Auswertung von Token-Nutzung ist erforderlich.
- MariaDB wird als relationale Datenbank eingesetzt.

## Sicherheits- und Betriebsrahmen

- Zero-trust intern: jedes Client-Request braucht gueltigen Bearer Token.
- Admin-Endpunkte akzeptieren nur gueltige OIDC JWTs mit passenden Rollen/Scopes.
- Tokenwerte werden nur gehasht gespeichert.
- Debug-Logging ist explizit per Token-Flag aktivierbar und revisionssicher protokolliert.
