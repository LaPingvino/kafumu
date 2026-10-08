// Esperanto dates where the browser has none (Chrome's built-in locale data
// lacks "eo" and falls back to US English): weekday and month names and a
// 24-hour clock, for the options Kafumu uses. Other languages are untouched.
(function () {
  try { if (Intl.DateTimeFormat.supportedLocalesOf(["eo"]).length) return; } catch (e) { return; }
  var DAYS = ["dimanĉo", "lundo", "mardo", "merkredo", "ĵaŭdo", "vendredo", "sabato"],
    DY = ["dim", "lun", "mar", "mer", "ĵaŭ", "ven", "sab"],
    MONTHS = ["januaro", "februaro", "marto", "aprilo", "majo", "junio", "julio", "aŭgusto", "septembro", "oktobro", "novembro", "decembro"],
    MO = ["jan", "feb", "mar", "apr", "maj", "jun", "jul", "aŭg", "sep", "okt", "nov", "dec"];
  function isEo(loc) {
    var l = Array.isArray(loc) ? loc[0] : loc;
    return l === "eo" || (l === undefined && document.documentElement.lang === "eo");
  }
  function two(n) { return (n < 10 ? "0" : "") + n; }
  function fmt(d, o, dateDefault, timeDefault) {
    o = o || {};
    var has = o.weekday || o.day || o.month || o.year || o.hour || o.minute, parts = [], date = [];
    if (!has) o = Object.assign({}, dateDefault ? { day: "numeric", month: "short", year: "numeric" } : {}, timeDefault ? { hour: "2-digit", minute: "2-digit" } : {});
    if (o.weekday) parts.push(o.weekday === "long" ? DAYS[d.getDay()] : DY[d.getDay()]);
    if (o.day) date.push(String(d.getDate()));
    if (o.month) date.push(o.month === "long" ? MONTHS[d.getMonth()] : o.month === "numeric" || o.month === "2-digit" ? String(d.getMonth() + 1) : MO[d.getMonth()]);
    if (o.year) date.push(String(d.getFullYear()));
    if (date.length) parts.push(date.join(" "));
    if (o.hour || o.minute) parts.push(two(d.getHours()) + ":" + two(d.getMinutes()));
    return parts.join(", ");
  }
  var P = Date.prototype, all = P.toLocaleString, day = P.toLocaleDateString, time = P.toLocaleTimeString;
  P.toLocaleString = function (loc, o) { return isEo(loc) ? fmt(this, o, true, true) : all.call(this, loc, o); };
  P.toLocaleDateString = function (loc, o) { return isEo(loc) ? fmt(this, o, true, false) : day.call(this, loc, o); };
  P.toLocaleTimeString = function (loc, o) { return isEo(loc) ? fmt(this, o, false, true) : time.call(this, loc, o); };
})();
