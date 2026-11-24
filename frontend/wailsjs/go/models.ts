export namespace config {
	
	export class Item {
	    id: string;
	    name: string;
	    path: string;
	    args: string;
	    work_dir: string;
	    icon: string;
	    admin: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.args = source["args"];
	        this.work_dir = source["work_dir"];
	        this.icon = source["icon"];
	        this.admin = source["admin"];
	    }
	}
	export class Group {
	    id: string;
	    name: string;
	    items: Item[];
	
	    static createFrom(source: any = {}) {
	        return new Group(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.items = this.convertValues(source["items"], Item);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Settings {
	    hotkey: string;
	    mouse_trigger: string;
	    mouse_swallow: boolean;
	    hide_on_launch: boolean;
	    hide_on_blur: boolean;
	    hide_on_esc: boolean;
	    hide_on_blur_strict: boolean;
	    click_mode: string;
	    auto_start: boolean;
	    start_hidden: boolean;
	    theme: string;
	    language: string;
	    show_recent: boolean;
	    collapsed_groups: string[];
	    window_positioned: boolean;
	    window_opacity: number;
	    window_width: number;
	    window_height: number;
	    window_x: number;
	    window_y: number;
	    font_scale: number;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hotkey = source["hotkey"];
	        this.mouse_trigger = source["mouse_trigger"];
	        this.mouse_swallow = source["mouse_swallow"];
	        this.hide_on_launch = source["hide_on_launch"];
	        this.hide_on_blur = source["hide_on_blur"];
	        this.hide_on_esc = source["hide_on_esc"];
	        this.hide_on_blur_strict = source["hide_on_blur_strict"];
	        this.click_mode = source["click_mode"];
	        this.auto_start = source["auto_start"];
	        this.start_hidden = source["start_hidden"];
	        this.theme = source["theme"];
	        this.language = source["language"];
	        this.show_recent = source["show_recent"];
	        this.collapsed_groups = source["collapsed_groups"];
	        this.window_positioned = source["window_positioned"];
	        this.window_opacity = source["window_opacity"];
	        this.window_width = source["window_width"];
	        this.window_height = source["window_height"];
	        this.window_x = source["window_x"];
	        this.window_y = source["window_y"];
	        this.font_scale = source["font_scale"];
	    }
	}
	export class Config {
	    settings: Settings;
	    groups: Group[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.settings = this.convertValues(source["settings"], Settings);
	        this.groups = this.convertValues(source["groups"], Group);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	

}

export namespace main {
	
	export class AppInfo {
	    version: string;
	    base_dir: string;
	    config_path: string;
	    config_backup: string;
	    icons_dir: string;
	    log_path: string;
	    exe_path: string;
	    auto_start: boolean;
	    auto_start_error: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.base_dir = source["base_dir"];
	        this.config_path = source["config_path"];
	        this.config_backup = source["config_backup"];
	        this.icons_dir = source["icons_dir"];
	        this.log_path = source["log_path"];
	        this.exe_path = source["exe_path"];
	        this.auto_start = source["auto_start"];
	        this.auto_start_error = source["auto_start_error"];
	    }
	}
	export class HotkeyStatus {
	    hotkey: string;
	    registered: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new HotkeyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hotkey = source["hotkey"];
	        this.registered = source["registered"];
	        this.error = source["error"];
	    }
	}
	export class RecentItem {
	    group_id: string;
	    group_name: string;
	    item_id: string;
	    name: string;
	    path: string;
	    icon: string;
	    admin: boolean;
	    count: number;
	    last_used: number;
	
	    static createFrom(source: any = {}) {
	        return new RecentItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.group_id = source["group_id"];
	        this.group_name = source["group_name"];
	        this.item_id = source["item_id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.icon = source["icon"];
	        this.admin = source["admin"];
	        this.count = source["count"];
	        this.last_used = source["last_used"];
	    }
	}
	export class SearchHit {
	    group_id: string;
	    group_name: string;
	    item_id: string;
	    name: string;
	    path: string;
	    args: string;
	    icon: string;
	    admin: boolean;
	    score: number;
	
	    static createFrom(source: any = {}) {
	        return new SearchHit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.group_id = source["group_id"];
	        this.group_name = source["group_name"];
	        this.item_id = source["item_id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.args = source["args"];
	        this.icon = source["icon"];
	        this.admin = source["admin"];
	        this.score = source["score"];
	    }
	}
	export class UpdateInfo {
	    current: string;
	    latest: string;
	    url: string;
	    notes: string;
	    available: boolean;
	    checked_at: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.url = source["url"];
	        this.notes = source["notes"];
	        this.available = source["available"];
	        this.checked_at = source["checked_at"];
	        this.error = source["error"];
	    }
	}

}

export namespace resolver {
	
	export class StartMenuEntry {
	    name: string;
	    path: string;
	    args: string;
	    work_dir: string;
	    lnk_path: string;
	    icon_src: string;
	    scope: string;
	
	    static createFrom(source: any = {}) {
	        return new StartMenuEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.args = source["args"];
	        this.work_dir = source["work_dir"];
	        this.lnk_path = source["lnk_path"];
	        this.icon_src = source["icon_src"];
	        this.scope = source["scope"];
	    }
	}

}

