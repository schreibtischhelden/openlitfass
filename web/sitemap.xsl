<?xml version="1.0" encoding="UTF-8"?>
<!-- openLitfass: macht sitemap.xml im Browser lesbar (Suchmaschinen
     ignorieren das Stylesheet). Zeigt die Termine wie die Liste der App:
     nach Tagen gruppiert, mit Uhrzeit, Kategorie, Ort und "Abgesagt".
     Die Termindaten stehen im eigenen Namensraum urn:openlitfass:sitemap
     (build-site, backend/internal/app/static.go). XSLT 1.0. -->
<xsl:stylesheet version="1.0"
    xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
    xmlns:s="http://www.sitemaps.org/schemas/sitemap/0.9"
    xmlns:olf="urn:openlitfass:sitemap"
    exclude-result-prefixes="s olf">

  <xsl:output method="html" encoding="UTF-8" indent="yes" doctype-system="about:legacy-compat"/>

  <xsl:variable name="site" select="/s:urlset/olf:site/@name"/>
  <!-- Links relativ zur Seite, damit sie auch in Vorschau und auf
       Spiegeln funktionieren (nicht immer zur öffentlichen Adresse). -->
  <xsl:variable name="base" select="concat(/s:urlset/olf:site/@base, '/')"/>
  <xsl:template name="href">
    <xsl:choose>
      <xsl:when test="starts-with(s:loc, $base)">./<xsl:value-of select="substring-after(s:loc, $base)"/></xsl:when>
      <xsl:otherwise><xsl:value-of select="s:loc"/></xsl:otherwise>
    </xsl:choose>
  </xsl:template>

  <xsl:template match="/">
    <html lang="de">
      <head>
        <meta charset="UTF-8"/>
        <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
        <meta name="robots" content="noindex"/>
        <title>Sitemap – <xsl:value-of select="$site"/></title>
        <link rel="icon" href="logo.svg" type="image/svg+xml"/>
        <link rel="stylesheet" href="css/tokens.css"/>
        <link rel="stylesheet" href="css/theme.css"/>
        <link rel="stylesheet" href="css/style.css"/>
      </head>
      <body>
        <div class="app static-page">
          <header class="app-header">
            <div class="app-header__inner">
              <a class="logo" href="./"><span><xsl:value-of select="$site"/></span></a>
            </div>
          </header>
          <main class="content">
            <section class="view">
              <h1 class="static-title">Sitemap</h1>
              <p class="result-count">
                <xsl:value-of select="/s:urlset/olf:site/@events"/> Termine ·
                Diese Datei ist für Suchmaschinen gedacht.
                <a href="termine/">Alle Termine</a> ·
                <a href="./">Zum Kalender</a>
              </p>

              <h2 class="day-heading static-day">Seiten</h2>
              <ul class="static-links">
                <xsl:for-each select="s:urlset/s:url[not(olf:event)]">
                  <li><a><xsl:attribute name="href"><xsl:call-template name="href"/></xsl:attribute><xsl:value-of select="s:loc"/></a></li>
                </xsl:for-each>
              </ul>

              <xsl:for-each select="s:urlset/s:url[olf:event]">
                <xsl:variable name="ev" select="olf:event"/>
                <!-- Neue Tagesüberschrift, wenn sich das Datum ändert (die
                     Einträge sind nach Datum sortiert). -->
                <xsl:if test="not(preceding-sibling::s:url[olf:event][1]/olf:event/@date = $ev/@date)">
                  <h2 class="day-heading static-day"><xsl:value-of select="$ev/@day"/></h2>
                </xsl:if>
                <a>
                  <xsl:attribute name="href"><xsl:call-template name="href"/></xsl:attribute>
                  <xsl:attribute name="class">event-card sitemap-card<xsl:if test="$ev/@cancelled = 'true'"> event-card--cancelled</xsl:if></xsl:attribute>
                  <div class="event-card__time">
                    <xsl:choose>
                      <xsl:when test="$ev/@time"><xsl:value-of select="$ev/@time"/></xsl:when>
                      <xsl:otherwise><small>ganztägig</small></xsl:otherwise>
                    </xsl:choose>
                  </div>
                  <div class="event-card__body">
                    <xsl:if test="$ev/@cancelled = 'true'"><span class="cancelled-badge">Abgesagt</span></xsl:if>
                    <xsl:if test="$ev/@category"><span class="category-label"><xsl:value-of select="$ev/@category"/></span></xsl:if>
                    <h3 class="event-card__title"><xsl:value-of select="$ev/@title"/></h3>
                    <xsl:if test="$ev/@place"><p class="event-card__meta"><span><xsl:value-of select="$ev/@place"/></span></p></xsl:if>
                  </div>
                </a>
              </xsl:for-each>
            </section>
            <footer class="site-footer">
              <a href="./">Zum Kalender</a>
              <a href="termine/">Alle Termine</a>
            </footer>
          </main>
        </div>
      </body>
    </html>
  </xsl:template>
</xsl:stylesheet>
