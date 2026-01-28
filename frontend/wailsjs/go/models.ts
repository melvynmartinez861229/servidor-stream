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
	    srtStreamName: string;
	    srtPort: number;
	    srtHost: string;
	    resolution: string;
	    frameRate: number;
	    status: string;
	    currentFile: string;
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
	        this.srtStreamName = source["srtStreamName"];
	        this.srtPort = source["srtPort"];
	        this.srtHost = source["srtHost"];
	        this.resolution = source["resolution"];
	        this.frameRate = source["frameRate"];
	        this.status = source["status"];
	        this.currentFile = source["currentFile"];
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
	    srtPrefix: string;
	    srtGroup: string;
	    defaultVideoPath: string;
	    logPath: string;
	    theme: string;
	    language: string;
	    maxLogLines: number;
	    videoEncoder: string;
	    encoderPreset: string;
	    encoderProfile: string;
	    encoderTune: string;
	    gopSize: number;
	    bFrames: number;
	    bitrateMode: string;
	    maxBitrate: string;
	    bufferSize: string;
	    crf: number;
	    srtLatency: number;
	    srtRecvBuffer: number;
	    srtSendBuffer: number;
	    srtOverheadBW: number;
	    srtPeerIdleTime: number;
	
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
	        this.srtPrefix = source["srtPrefix"];
	        this.srtGroup = source["srtGroup"];
	        this.defaultVideoPath = source["defaultVideoPath"];
	        this.logPath = source["logPath"];
	        this.theme = source["theme"];
	        this.language = source["language"];
	        this.maxLogLines = source["maxLogLines"];
	        this.videoEncoder = source["videoEncoder"];
	        this.encoderPreset = source["encoderPreset"];
	        this.encoderProfile = source["encoderProfile"];
	        this.encoderTune = source["encoderTune"];
	        this.gopSize = source["gopSize"];
	        this.bFrames = source["bFrames"];
	        this.bitrateMode = source["bitrateMode"];
	        this.maxBitrate = source["maxBitrate"];
	        this.bufferSize = source["bufferSize"];
	        this.crf = source["crf"];
	        this.srtLatency = source["srtLatency"];
	        this.srtRecvBuffer = source["srtRecvBuffer"];
	        this.srtSendBuffer = source["srtSendBuffer"];
	        this.srtOverheadBW = source["srtOverheadBW"];
	        this.srtPeerIdleTime = source["srtPeerIdleTime"];
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

