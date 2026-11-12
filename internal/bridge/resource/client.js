// LatchAdmin bridge (client): draws notices in a see-through web view that never takes focus, so the game keeps
// its controls. Installed and updated by LatchAdmin; edits here are overwritten.
"use strict";

(function () {
  const URL = "fw://resources/latchadmin/ui/index.html";
  let view = -1;
  let ready = false;
  const queue = [];

  function open() {
    if (view !== -1) return;
    try {
      view = Web.createView(URL, { visible: false, focus: false, zIndex: 900 });
    } catch (e) {
      console.log("Can't open the notice view: " + e);
      view = -2;
      return;
    }
    if (view < 0) return;
    Web.on(view, "ui:ready", function () {
      ready = true;
      while (queue.length) deliver(queue.shift());
    });
    Web.on(view, "ui:idle", function () {
      try { Web.hideView(view); } catch (e) {}
    });
  }

  function deliver(notice) {
    try {
      Web.showView(view);
      Web.emit(view, "latchadmin:notice", notice);
    } catch (e) {
      console.log("Can't show a notice: " + e);
    }
  }

  Events.on("latchadmin:notice", function (notice) {
    if (!notice || typeof notice !== "object") return;
    open();
    if (view < 0) {
      // No web view on this client: fall back to the game's own notification if there is one.
      try { if (typeof Game !== "undefined" && Game.notify) Game.notify(String(notice.title || "") + (notice.text ? " - " + notice.text : "")); } catch (e) {}
      return;
    }
    if (ready) deliver(notice);
    else queue.push(notice);
  });
})();
