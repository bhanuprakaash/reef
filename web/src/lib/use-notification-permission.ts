import { useState } from 'react';

export function useNotificationPermission() {
  const [permission, setPermission] = useState(Notification.permission);
  return {
    permission,
    request: () => void Notification.requestPermission().then(setPermission),
  };
}
