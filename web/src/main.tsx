import { render } from "@solidjs/web";
import "../styles/tokens.css";
import "../styles/base.css";
import "../styles/components.css";
import { App } from "./app";
import { loadRoles } from "./roles";
import { startRouter } from "./router";
import { takeSharedLink } from "./links";

startRouter();
takeSharedLink();
await loadRoles();
render(() => <App />, document.getElementById("app")!);
