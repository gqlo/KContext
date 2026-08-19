package kcontext

import "html/template"

const headerIntroHTML = `<p class="subtitle">OpenShift Virtualization Performance and Scale</p>
<p class="about"><strong>What it is:</strong> KContext is an alert triage dashboard with <strong>persistent alert history</strong> for OpenShift and Kubernetes clusters — not just what Alertmanager shows right now.</p>
<p class="does"><strong>What it does:</strong> Ingests firing and resolved alerts from Alertmanager (poll or webhook) and <strong>stores them in Redis</strong> so incidents survive restarts and stay searchable for future debugging. Filter past fires by severity, namespace, status, and time; cluster version info in the sidebar.</p>
<p class="repo">Source: <a href="https://github.com/gqlo/KContext" target="_blank" rel="noopener noreferrer">github.com/gqlo/KContext</a></p>`

const utcClockHTML = `<div class="utc-clock" title="Current UTC time" aria-label="Current UTC time">
  <span class="utc-clock-label">UTC</span>
  <time id="utc-clock-time" class="utc-clock-time" datetime=""></time>
</div>`

func headerIntro() template.HTML {
	return template.HTML(headerIntroHTML)
}

func utcClock() template.HTML {
	return template.HTML(utcClockHTML)
}
