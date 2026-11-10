import i18n from "i18next"
import LanguageDetector from "i18next-browser-languagedetector"
import { initReactI18next } from "react-i18next"
import en from "@/locales/en"
import tr from "@/locales/tr"

export const LANGUAGES = [
  { id: "en", label: "English" },
  { id: "tr", label: "Türkçe" },
]

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: { en: { translation: en }, tr: { translation: tr } },
    fallbackLng: "en",
    supportedLngs: ["en", "tr"],
    nonExplicitSupportedLngs: true,
    interpolation: { escapeValue: false },
    detection: { order: ["localStorage", "navigator"], lookupLocalStorage: "latchadmin-lang", caches: ["localStorage"] },
  })

export default i18n
