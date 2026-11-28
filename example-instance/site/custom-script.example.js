// Example: rename to custom-script.js to load it on every page (start page
// and, with static pages enabled, event, list and place pages).
// Without custom-script.js, no script tag is added.
//
// This example loads a self-hosted, cookie-free statistics tool such as
// Plausible or Umami. Replace the URL and the domain with your own, describe
// the tool in site/datenschutz.html and, if your hosting sends a Content
// Security Policy, allow its origin there.
const s = document.createElement("script");
s.defer = true;
s.src = "https://stats.example.events/js/script.js";
s.dataset.domain = "example.events";
document.head.appendChild(s);
