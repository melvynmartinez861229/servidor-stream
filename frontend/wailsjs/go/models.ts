export namespace app {
	
	export class LogEntry {
	    timestamp: string;
	    level: string;
	    message: string;
	    channelId?: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = source["timestamp"];
	        this.level = source["level"];
	        this.message = source["message"];
	        this.channelId = source["channelId"];
	    }
	}

}

export namespace channel {
	
	export class Stats {
	    framesProcessed: number;
	    bytesSent: number;
	    uptime: number;
	    lastError?: string;
	    errorCount: number;
	
	    static createFrom(source: any = {}) {
	        return new Stats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.framesProcessed = source["framesProcessed"];
	        this.bytesSent = source["bytesSent"];
	        this.uptime = source["uptime"];
	        this.lastError = source["lastError"];
	        this.errorCount = source["errorCount"];
	    }
	}
	export class Channel {
	    id: string;
	    label: string;
	    videoPath: string;
	    ndiStreamName: string;
	    srtPort: number;
	    status: string;
	    previewEnabled: boolean;
	    currentFile: string;
	    previewBase64?: string;
	    // Go type: time
	    lastPreviewUpdate: any;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    errorMessage?: string;
	    stats: Stats;
	
	    static createFrom(source: any = {}) {
	        return new Channel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.videoPath = source["videoPath"];
	        this.ndiStreamName = source["ndiStreamName"];
	        this.srtPort = source["srtPort"];
	        this.status = source["status"];
	        this.previewEnabled = source["previewEnabled"];
	        this.currentFile = source["currentFile"];
	        this.previewBase64 = source["previewBase64"];
	        this.lastPreviewUpdate = this.convertValues(source["lastPreviewUpdate"], null);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.errorMessage = source["errorMessage"];
	        this.stats = this.convertValues(source["stats"], Stats);
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

export namespace config {
	
	export class Config {
	    webSocketPort: number;
	    ffmpegPath: string;
	    autoRestart: boolean;
	    defaultVideoBitrate: string;
	    defaultAudioBitrate: string;
	    defaultFrameRate: number;
	    testPatternPath: string;
	    previewConfig: preview.Config;
	    ndiPrefix: string;
	    ndiGroup: string;
	    defaultVideoPath: string;
	    logPath: string;
	    theme: string;
	    language: string;
	    maxLogLines: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.webSocketPort = source["webSocketPort"];
	        this.ffmpegPath = source["ffmpegPath"];
	        this.autoRestart = source["autoRestart"];
	        this.defaultVideoBitrate = source["defaultVideoBitrate"];
	        this.defaultAudioBitrate = source["defaultAudioBitrate"];
	        this.defaultFrameRate = source["defaultFrameRate"];
	        this.testPatternPath = source["testPatternPath"];
	        this.previewConfig = this.convertValues(source["previewConfig"], preview.Config);
	        this.ndiPrefix = source["ndiPrefix"];
	        this.ndiGroup = source["ndiGroup"];
	        this.defaultVideoPath = source["defaultVideoPath"];
	        this.logPath = source["logPath"];
	        this.theme = source["theme"];
	        this.language = source["language"];
	        this.maxLogLines = source["maxLogLines"];
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

export namespace preview {
	
	export class Config {
	    width: number;
	    height: number;
	    quality: number;
	    updateIntervalMs: number;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.width = source["width"];
	        this.height = source["height"];
	        this.quality = source["quality"];
	        this.updateIntervalMs = source["updateIntervalMs"];
	        this.enabled = source["enabled"];
	    }
	}

}

export namespace websocket {
	
	export class ClientInfo {
	    id: string;
	    name: string;
	    // Go type: time
	    connectedAt: any;
	    // Go type: time
	    lastMessageAt: any;
	    messageCount: number;
	    remoteAddr: string;
	
	    static createFrom(source: any = {}) {
	        return new ClientInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.connectedAt = this.convertValues(source["connectedAt"], null);
	        this.lastMessageAt = this.convertValues(source["lastMessageAt"], null);
	        this.messageCount = source["messageCount"];
	        this.remoteAddr = source["remoteAddr"];
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

