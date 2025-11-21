// RelativeTime Web Component
//
//     Progressive enhancement for <time datetime> elements that uses
//     Javascript's Intl.RelativeTimeFormat to display relative time in a
//     human-readable format.
//
// Usage:
//
//     Wrap a <time> element with a <relative-time> element to display the
//     time relative to the browser's current time.
//
//     <relative-time>
//         <time datetime="2025-05-23T15:00:00-07:00">
//             2025-05-23T15:00:00-07:00
//         </time>
//     </relative-time>
//
//     The <time> element's datetime attribute is used to determine the
//     relative time. The text content of the <time> element is used as a
//     fallback if the datetime attribute is not present, if the datetime
//     attribute is invalid, or if the component is not loaded.
//
//     To load the component, include this script in your HTML:
//
//     <script src="/js/components/relative-time.js"></script>
//
// Options
//
//     format (string)
//         The format of the relative time string. Defaults to "long".
//         Supported formats are "long", "short", and "narrow".
//
// By Herd Works, Inc (https://herd.works)
class RelativeTime extends HTMLElement {
    // static properties
    static tagName = "relative-time";

    // instance properties
    timestamp;
    format;

    // lifecycle methods
    constructor() { super() };
    connectedCallback() {
        this.format = this.querySelector("format") || "long";
        let element = this.querySelector("time");
        if (!(element instanceof HTMLElement)) { return };
        element.setAttribute("title", this.date);
        element.innerText = this.relativeTime(this.date);
    };

    // getters
    get datetime() {
        let date = this.querySelector("[datetime]");
        if (!date) { return null };
        return date.getAttribute("datetime");
    }
    get date() {
        if (!this.timestamp) { this.timestamp = this.datetime };
        if (!this.timestamp) { this.timestamp = Date.now() };
        return new Date(this.timestamp);
    }

    relativeTime(date=null) {
        if (date == null) { return "in the future" };
        var rtf = new Intl.RelativeTimeFormat('en', { style: this.format, numeric: 'auto' });
        var diff = Math.round((new Date() - date) / 1000); // round to the nearest second
        var future = diff < 0;
        var offset = Math.abs(diff); // absolute value of the difference
        var unit;
        switch(true) {
            case (offset > 31536000):
                offset = (Math.round(offset/31536000)); // round to the nearest number of years
                unit = "year";
                break;
            case (offset > 2592000):
                offset = (Math.round(offset/2592000)); // round to the nearest number of months
                unit = "month";
                break;
            case (offset > 604800):
                offset = (Math.round(offset/604800)); // round to the nearest number of weeks
                unit = "week";
                break;
            case (offset > 86400):
                offset = (Math.round(offset/86400)); // round to the nearest number of days
                unit = "day";
                break;
            case (offset > 3600):
                offset = (Math.round(offset/3600)); // round to the nearest number of hours
                unit = "hour";
                break;
            case (offset > 60):
                offset = (Math.round(offset/60)); // round to the nearest number of minutes
                unit = "minute";
                break;
            case (offset > 0):
            default:
                offset = Math.round(offset); // round to the nearest number of seconds
                unit = "second";
                break;
        };
        if (future) { offset = -offset }; // negate the difference if the date is in the future
        var relativeTime = rtf.format(-offset, unit);
        return relativeTime;
    };

    // static methods
    static register() {
        if (!this.tagName) { console.debug(`component ${this.name}.tagName is required for registration`); return }; // guard
        if (customElements.get(this.tagName) || customElements.getName(this)) { return }; // guard
        if (document.readyState == "complete" || document.readyState == "interactive") {
            customElements.define(this.tagName, this);
        } else {
            document.addEventListener("DOMContentLoaded", customElements.define(this.tagName, this));
        };
        console.debug(`component "${this.tagName}" registered`);
    };

};
RelativeTime.register();
