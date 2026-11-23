// The editor is usable without JavaScript. This small enhancement warns about
// unsaved changes and does not transmit data or alter validation behavior.
(() => {
  "use strict";
  const form = document.querySelector("form[data-dirty-check]");
  if (!form) return;
  const snapshot = () => new URLSearchParams(new FormData(form)).toString();
  const initial = snapshot();
  let submitting = false;
  window.addEventListener("beforeunload", (event) => {
    if (submitting || snapshot() === initial) return;
    event.preventDefault();
    event.returnValue = "";
  });
  form.addEventListener("submit", () => {
    submitting = true;
  });
})();
