import { useEffect } from 'react';
import { useLocation } from 'react-router';
import { useTranslation } from 'react-i18next';

import { PANEL_ROUTES } from '@/routes/panelRoutes';

export function usePageTitle() {
  const { pathname } = useLocation();
  const { t } = useTranslation();

  useEffect(() => {
    const route = PANEL_ROUTES.find((item) => item.path === pathname);
    const title = route ? (route.title ?? t(route.titleKey)) : '3X-UI';
    const host = window.location.hostname;
    document.title = host ? `${host} - ${title}` : title;
  }, [pathname, t]);
}
