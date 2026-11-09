import { AppSwitcher } from '@appthrust/app-launcher'
import { FlaskConical, Gauge, Languages, ListOrdered, Plug, Server } from 'lucide-react'
import type { ReactNode } from 'react'
import type { MessageId } from '../copy'
import { setLanguage, useText } from '../i18n'
import { paths } from '../routes/paths'
import { Button, HrefLink } from '../ui'

const NAV_LINK =
  'flex min-h-10 min-w-10 items-center justify-center gap-1.5 rounded-md px-2.5 text-sm font-medium whitespace-nowrap text-stone-600 hover:bg-stone-100 hover:text-stone-900 md:min-h-9 [&_svg]:size-4 [&_svg]:shrink-0'
const NAV_ACTIVE = { className: 'bg-accent-soft text-accent-strong hover:bg-accent-soft hover:text-accent-strong' }
// Five sections, the language switch and the app switcher fit one line only from `lg`: below it the section links
// show their icon and keep the label for screen readers and as a tooltip.
const NAV_LABEL = 'max-lg:sr-only'

const SECTIONS: { href: string; label: MessageId; icon: ReactNode }[] = [
  { href: paths.overview, label: 'aiGateway.nav.overview', icon: <Gauge aria-hidden /> },
  { href: paths.providers, label: 'aiGateway.nav.providers', icon: <Server aria-hidden /> },
  { href: paths.playground, label: 'aiGateway.nav.playground', icon: <FlaskConical aria-hidden /> },
  { href: paths.invocations, label: 'aiGateway.nav.invocations', icon: <ListOrdered aria-hidden /> },
  { href: paths.connect, label: 'aiGateway.nav.connect', icon: <Plug aria-hidden /> },
]

/** Top bar (brand, sections, language, the app switcher) over the page. */
export function Shell({ children }: { children: ReactNode }) {
  const { t, language } = useText()
  const other = language === 'ja' ? 'en' : 'ja'
  return (
    <div className="min-h-dvh">
      <a
        href="#main"
        className="sr-only z-50 min-h-10 content-center rounded-md bg-white px-3 py-2 text-sm font-medium shadow focus:not-sr-only focus:fixed focus:top-2 focus:left-2"
      >
        {t('aiGateway.nav.skipToMain')}
      </a>
      <header className="sticky top-0 z-10 border-b border-line bg-white/90 backdrop-blur supports-[backdrop-filter]:bg-white/80">
        <div className="mx-auto flex h-14 max-w-5xl items-center gap-1 px-3 sm:gap-2 sm:px-6">
          <HrefLink href={paths.overview} className="mr-2 flex shrink-0 items-center gap-2.5 text-[15px] font-semibold tracking-tight text-stone-900">
            <span aria-hidden className="grid size-6 place-items-center rounded-md bg-accent text-[10px] font-bold text-white">
              {t('aiGateway.productMark')}
            </span>
            <span className="max-md:sr-only">{t('aiGateway.product')}</span>
          </HrefLink>
          <nav aria-label={t('aiGateway.nav.label')} className="flex min-w-0 items-center gap-1">
            {SECTIONS.map((section) => (
              <HrefLink
                key={section.href}
                href={section.href}
                title={t(section.label)}
                className={NAV_LINK}
                activeProps={NAV_ACTIVE}
                activeOptions={{ includeSearch: false }}
              >
                {section.icon}
                <span className={NAV_LABEL}>{t(section.label)}</span>
              </HrefLink>
            ))}
          </nav>
          <div className="ml-auto flex shrink-0 items-center gap-1 sm:gap-2">
            <Button
              tone="ghost"
              size="sm"
              icon={<Languages />}
              aria-label={`${t('aiGateway.nav.language')}: ${t(other === 'en' ? 'aiGateway.language.en' : 'aiGateway.language.ja')}`}
              onClick={() => setLanguage(other)}
            >
              <span lang={other} className="hidden sm:inline">
                {t(other === 'en' ? 'aiGateway.language.en' : 'aiGateway.language.ja')}
              </span>
            </Button>
            <AppSwitcher language={language} />
          </div>
        </div>
      </header>
      <main id="main" tabIndex={-1} className="mx-auto w-full max-w-5xl px-4 py-6 focus:outline-none sm:px-6 md:py-8">
        {children}
      </main>
    </div>
  )
}
