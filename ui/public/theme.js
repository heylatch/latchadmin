// Sets the theme class before the first paint (an external file, so the CSP needs no 'unsafe-inline').
try {
  var t = localStorage.getItem("latchadmin-theme") || "system"
  var dark = t === "dark" || (t === "system" && matchMedia("(prefers-color-scheme: dark)").matches)
  document.documentElement.classList.toggle("dark", dark)
  document.documentElement.style.colorScheme = dark ? "dark" : "light"
} catch (e) {}
