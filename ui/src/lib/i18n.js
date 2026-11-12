import i18n from "i18next"
import LanguageDetector from "i18next-browser-languagedetector"
import { initReactI18next } from "react-i18next"
import de from "@/locales/de"
import en from "@/locales/en"
import es from "@/locales/es"
import fr from "@/locales/fr"
import it from "@/locales/it"
import pt from "@/locales/pt"
import ru from "@/locales/ru"
import tr from "@/locales/tr"

export const LANGUAGES = [
  { id: "en", label: "English" },
  { id: "de", label: "Deutsch" },
  { id: "es", label: "Español" },
  { id: "fr", label: "Français" },
  { id: "it", label: "Italiano" },
  { id: "pt", label: "Português" },
  { id: "ru", label: "Русский" },
  { id: "tr", label: "Türkçe" },
]

const resources = Object.fromEntries(Object.entries({ en, de, es, fr, it, pt, ru, tr }).map(([id, translation]) => [id, { translation }]))

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: "en",
    supportedLngs: LANGUAGES.map((l) => l.id),
    nonExplicitSupportedLngs: true,
    interpolation: { escapeValue: false },
    detection: { order: ["localStorage", "navigator"], lookupLocalStorage: "latchadmin-lang", caches: ["localStorage"] },
  })

export default i18n
