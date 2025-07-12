export namespace main {
	
	export class ClipboardResponse {
	    content: string;
	    timestamp: string;
	
	    static createFrom(source: any = {}) {
	        return new ClipboardResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.content = source["content"];
	        this.timestamp = source["timestamp"];
	    }
	}

}

