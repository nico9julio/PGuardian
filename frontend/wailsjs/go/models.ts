export namespace main {
	
	export class APIResponse {
	    status: number;
	    body: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new APIResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.body = source["body"];
	        this.error = source["error"];
	    }
	}

}

