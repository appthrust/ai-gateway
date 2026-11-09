// All gateway wording, as dashboard-i18n catalogs: the English source is the
// `defaultMessage`, Japanese (the default language) is `translations.ja`.
// Screens read it through `useText().t(id, values)` (src/i18n.tsx); no screen
// hardcodes user-facing text. English sources must be unique across catalogs.
import type { DashboardMessageCatalog } from '@appthrust/platform-dashboard-i18n'
import { commonMessages } from './common'
import { connectMessages } from './connect'
import { invocationMessages } from './invocations'
import { overviewMessages } from './overview'
import { playgroundMessages } from './playground'
import { providerMessages } from './providers'

export const catalogs: readonly DashboardMessageCatalog[] = [commonMessages, overviewMessages, providerMessages, playgroundMessages, invocationMessages, connectMessages]

export type MessageId =
  | keyof typeof commonMessages
  | keyof typeof overviewMessages
  | keyof typeof providerMessages
  | keyof typeof playgroundMessages
  | keyof typeof invocationMessages
  | keyof typeof connectMessages
