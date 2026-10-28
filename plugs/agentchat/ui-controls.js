// Shared anchored menu behavior. Call dispose when the owning add-on is removed.
export function actionMenu(popup, triggers) {
  let active = null;
  popup.setAttribute('role', 'menu');
  for (const trigger of triggers) {
    trigger.setAttribute('aria-haspopup', 'menu');
    trigger.setAttribute('aria-controls', popup.id);
    trigger.setAttribute('aria-expanded', 'false');
  }
  function close(restore = true) {
    popup.hidden = true;
    const previous = active;
    active = null;
    for (const trigger of triggers) trigger.setAttribute('aria-expanded', 'false');
    if (restore) previous?.focus();
  }
  function toggle(trigger, render) {
    if (active === trigger && !popup.hidden) { close(); return; }
    close(false);
    render();
    active = trigger;
    popup.hidden = false;
    trigger.setAttribute('aria-expanded', 'true');
    const items = popup.querySelectorAll('button:not(:disabled)');
    for (const item of items) item.setAttribute('role', 'menuitem');
    items[0]?.focus();
  }
  const pointer = event => {
    if (!active) return;
    const path = event.composedPath();
    if (!path.includes(popup) && !triggers.some(t => path.includes(t))) close();
  };
  const key = event => {
    if (!active) return;
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(); return; }
    if (event.key === 'Tab') { close(false); return; }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault(); event.stopPropagation();
    const items = [...popup.querySelectorAll('button:not(:disabled)')];
    if (!items.length) return;
    const current = items.indexOf(event.composedPath()[0]);
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 :
      (current + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
    items[next].focus();
  };
  document.addEventListener('pointerdown', pointer);
  popup.addEventListener('keydown', key);
  return { toggle, close, dispose() { close(false); document.removeEventListener('pointerdown', pointer); popup.removeEventListener('keydown', key); } };
}
