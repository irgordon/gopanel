(() => {
  "use strict";

  const minimumErrorStatus = 400;
  const maximumErrorStatus = 599;

  document.addEventListener("htmx:beforeSwap", allowSafeErrorSwap);
  document.addEventListener("htmx:sendError", showConnectionFailure);
  document.addEventListener("htmx:timeout", showTimeoutFailure);

  function allowSafeErrorSwap(event) {
    const xhr = event.detail && event.detail.xhr;
    if (!xhr || !isErrorStatus(xhr.status) || !isHTMLResponse(xhr)) {
      return;
    }

    event.detail.shouldSwap = true;
  }

  function showConnectionFailure(event) {
    showTransportFailure(
      event,
      "GoPanel could not be reached.",
      "Check your connection, then reload GoPanel and try again.",
    );
  }

  function showTimeoutFailure(event) {
    showTransportFailure(
      event,
      "GoPanel did not respond in time.",
      "Reload GoPanel and try again. If the problem continues, check the server.",
    );
  }

  function showTransportFailure(event, title, guidance) {
    const target = requestTarget(event);
    if (!target) {
      return;
    }

    target.replaceChildren(transportAlert(title, guidance));
  }

  function requestTarget(event) {
    const detail = event.detail || {};
    if (detail.target instanceof Element) {
      return detail.target;
    }

    if (!(detail.elt instanceof Element)) {
      return null;
    }

    return detail.elt.closest("[data-request-region]") || detail.elt;
  }

  function transportAlert(title, guidance) {
    const alert = document.createElement("div");
    alert.className = "rounded-2xl border border-rose-300/25 bg-rose-300/10 p-5 text-rose-50";
    alert.setAttribute("role", "alert");
    alert.setAttribute("data-transport-error", "true");

    const heading = document.createElement("p");
    heading.className = "font-semibold";
    heading.textContent = title;

    const message = document.createElement("p");
    message.className = "mt-2 text-sm leading-6 text-rose-100/80";
    message.textContent = guidance;

    const reloadLink = document.createElement("a");
    reloadLink.className = "mt-4 inline-flex min-h-11 items-center rounded-xl border border-rose-200/25 px-4 py-2 text-sm font-semibold text-white hover:bg-white/10";
    reloadLink.href = "/";
    reloadLink.textContent = "Reload GoPanel";

    alert.append(heading, message, reloadLink);
    return alert;
  }

  function isErrorStatus(status) {
    return status >= minimumErrorStatus && status <= maximumErrorStatus;
  }

  function isHTMLResponse(xhr) {
    const contentType = xhr.getResponseHeader("Content-Type") || "";
    return contentType.toLowerCase().startsWith("text/html");
  }
})();
