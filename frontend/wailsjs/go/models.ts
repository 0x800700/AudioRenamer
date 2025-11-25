export namespace main {
	
	export class AIParsedTrack {
	    original_filename: string;
	    artist: string;
	    title: string;
	    track_number: string;
	
	    static createFrom(source: any = {}) {
	        return new AIParsedTrack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.original_filename = source["original_filename"];
	        this.artist = source["artist"];
	        this.title = source["title"];
	        this.track_number = source["track_number"];
	    }
	}
	export class LocalTrack {
	    path: string;
	    originalName: string;
	    tagArtist: string;
	    tagTitle: string;
	
	    static createFrom(source: any = {}) {
	        return new LocalTrack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.originalName = source["originalName"];
	        this.tagArtist = source["tagArtist"];
	        this.tagTitle = source["tagTitle"];
	    }
	}
	export class MatchedTrack {
	    localPath: string;
	    originalName: string;
	    proposedNewName: string;
	    confidence: number;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new MatchedTrack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.localPath = source["localPath"];
	        this.originalName = source["originalName"];
	        this.proposedNewName = source["proposedNewName"];
	        this.confidence = source["confidence"];
	        this.status = source["status"];
	    }
	}

}

