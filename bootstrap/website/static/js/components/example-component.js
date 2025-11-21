class ExampleComponent extends HTMLElement {

    // static attributes
    static tagName = "example-component";
    static observedAttributes = ["name"];

    // attributes
    name;

    // lifecycle methods
    constructor() {
        super();
        this.addEventListener("click", this); // this.handleEvent(event)
    };
    connectedCallback() {};
    disconnectedCallback() {};
    adoptedCallback() {};
    attributeChangedCallback(name, oldValue, newValue) {
        if ( newValue == null || oldValue == newValue ) { return } // guard
        console.debug(`attribute "${name}" changed`)
        return
    };

    // event handler methods
    handleEvent(event) {
        console.debug(`handling "${event.type}" event`);
        this[`on${event.type}`](event);
    };
    onclick(event) {
        console.debug(`clicked ${event.target} element`);
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
ExampleComponent.register();