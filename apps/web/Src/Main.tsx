import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import { App } from '@/App/App';
import { loadBootMessages } from '@/I18n/BootCatalog';
import { readStoredLocale } from '@/I18n/Locale';
import { takeInviteTokenFromLocation } from '@/Modules/Identity/Store/InviteToken';
import { takeSetupTokenFromLocation } from '@/Modules/Identity/Store/SetupToken';
import { takeOidcCallbackFromLocation } from '@/Modules/Session/Store/OidcCallback';
import '@/Styles/Fonts.css';
import '@/Styles/Globals.css';

takeSetupTokenFromLocation();
takeInviteTokenFromLocation();
takeOidcCallbackFromLocation();

const container = document.getElementById('root');
if (!container) throw new Error('root element is missing');

const render = () => {
  createRoot(container).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
};

loadBootMessages(readStoredLocale()).then(render, render);
