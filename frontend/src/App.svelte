<script>
  import {
    SelectFolder,
    GenerateTemplateRenames,
    FetchAndMatchTracks,
    RenameMatchedTracks,
    ParseFilenamesWithAI,
  } from "../wailsjs/go/main/App";
  import { onMount } from "svelte";
  import logo from "./assets/ProBablyWorks.png";

  let localTracks = [];
  let processedTracks = [];
  let bandcampUrl = "";
  let notification = "";
  let isLoading = false;
  let folderPath = "";
  let apiKey = localStorage.getItem("openai_api_key") || "";
  let showSettings = false;

  async function selectFolder() {
    try {
      isLoading = true;
      notification = "Scanning folder...";
      const result = await SelectFolder();
      localTracks = result || [];

      if (localTracks.length > 0) {
        const firstTrackPath = localTracks[0].path;
        // Simple heuristic to get folder path
        const lastSeparatorIndex = Math.max(
          firstTrackPath.lastIndexOf("/"),
          firstTrackPath.lastIndexOf("\\"),
        );
        folderPath = firstTrackPath.substring(0, lastSeparatorIndex);
        notification = `Found ${localTracks.length} local files.`;

        processedTracks = localTracks.map((track) => ({
          localPath: track.path,
          originalName: track.originalName,
          proposedNewName: track.originalName,
          confidence: 0,
          status: "Original",
        }));
      } else {
        folderPath = "";
        notification = "No audio files found in the selected directory.";
        processedTracks = [];
      }
    } catch (error) {
      handleError(error);
    } finally {
      isLoading = false;
    }
  }

  let templateFormat = "Track. Artist - Title";

  async function generateFromTemplate() {
    try {
      isLoading = true;
      notification = "Generating names from template...";
      processedTracks =
        (await GenerateTemplateRenames(localTracks, templateFormat)) || [];
      notification = `Generated ${processedTracks.length} names. Review and rename.`;
    } catch (error) {
      handleError(error);
    } finally {
      isLoading = false;
    }
  }

  async function fetchAndMatch() {
    if (!bandcampUrl) {
      notification = "Please enter a Bandcamp URL.";
      return;
    }
    try {
      isLoading = true;
      notification = "Fetching data and matching files...";
      processedTracks =
        (await FetchAndMatchTracks(bandcampUrl, localTracks)) || [];
      notification = `Matched ${processedTracks.length} tracks. Review and rename.`;
    } catch (error) {
      handleError(error);
    } finally {
      isLoading = false;
    }
  }

  async function parseWithAI() {
    if (!apiKey) {
      notification = "Please set your API Key in settings first.";
      showSettings = true;
      return;
    }
    try {
      isLoading = true;
      notification = "Asking AI to parse filenames...";

      const filenames = localTracks.map((t) => t.originalName);
      const aiResults = await ParseFilenamesWithAI(filenames, apiKey);

      // Map AI results back to processedTracks
      processedTracks = localTracks.map((track) => {
        const aiMatch = aiResults.find(
          (r) => r.original_filename === track.originalName,
        );
        let proposedName = track.originalName;
        let status = "AI Parsed";
        let confidence = 0.9; // Assume high confidence if AI returns it

        if (aiMatch) {
          // Construct new name: "01. Artist - Title.ext"
          const ext = track.originalName.split(".").pop();
          const trackNum = aiMatch.track_number
            ? aiMatch.track_number.padStart(2, "0")
            : "00";
          proposedName = `${trackNum}. ${aiMatch.artist} - ${aiMatch.title}.${ext}`;
        } else {
          status = "AI Failed";
          confidence = 0.1;
        }

        return {
          localPath: track.path,
          originalName: track.originalName,
          proposedNewName: proposedName,
          confidence: confidence,
          status: status,
        };
      });

      notification = `AI parsing complete. Review and rename.`;
    } catch (error) {
      handleError(error);
    } finally {
      isLoading = false;
    }
  }

  async function renameFiles() {
    try {
      isLoading = true;
      notification = "Renaming files...";
      const result = await RenameMatchedTracks(processedTracks);
      notification = result;
      // Reset state after renaming
      localTracks = [];
      processedTracks = [];
      folderPath = "";
    } catch (error) {
      handleError(error);
    } finally {
      isLoading = false;
    }
  }

  function handleProposedNameChange(event, index) {
    processedTracks[index].proposedNewName = event.target.value;
  }

  function handleError(error) {
    console.error(error);
    notification = `Error: ${error.message || error}`;
  }

  function saveSettings() {
    localStorage.setItem("openai_api_key", apiKey);
    showSettings = false;
    notification = "Settings saved.";
  }

  function getConfidenceColor(confidence) {
    if (confidence < 0.4) return "bg-red-500/20 text-red-200 border-red-500/50";
    if (confidence < 0.8)
      return "bg-yellow-500/20 text-yellow-200 border-yellow-500/50";
    return "bg-green-500/20 text-green-200 border-green-500/50";
  }
</script>

<main
  class="h-screen flex flex-col bg-gray-900 text-gray-100 font-sans selection:bg-blue-500 selection:text-white overflow-hidden"
>
  <div
    class="flex-1 flex flex-col min-h-0 max-w-6xl mx-auto w-full p-4 md:p-8 space-y-8"
  >
    <!-- Header -->
    <header
      class="flex-none flex flex-wrap gap-4 justify-between items-center border-b border-gray-700 pb-6"
    >
      <div class="flex items-center gap-6">
        <div>
          <h1
            class="text-3xl font-bold bg-gradient-to-r from-blue-400 to-purple-500 bg-clip-text text-transparent"
          >
            AudioRenamer
          </h1>
          <p class="text-gray-400 text-sm mt-1">
            Smart audio file organization
          </p>
        </div>

        <button
          on:click={() => (showSettings = !showSettings)}
          class="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-gray-800 hover:bg-gray-700 transition-colors text-gray-400 hover:text-white border border-gray-700"
          title="Settings"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
            />
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
            />
          </svg>
          <span class="text-xs font-mono font-medium">API-KEY</span>
        </button>
      </div>

      <div class="flex flex-col items-end">
        <img src={logo} alt="ProBably Works" class="h-24 object-contain" />
      </div>
    </header>

    <!-- Settings Modal -->
    {#if showSettings}
      <div
        class="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50"
        on:click|self={() => (showSettings = false)}
      >
        <div
          class="bg-gray-800 p-6 rounded-xl shadow-2xl border border-gray-700 w-full max-w-md transform transition-all"
        >
          <h2 class="text-xl font-bold mb-4 text-white">Settings</h2>
          <div class="space-y-4">
            <div>
              <label class="block text-sm font-medium text-gray-400 mb-1"
                >Google AI API Key</label
              >
              <input
                type="password"
                bind:value={apiKey}
                placeholder="AIza..."
                class="w-full bg-gray-900 border border-gray-700 rounded-lg px-4 py-2 text-white focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none transition-all"
              />
              <p class="text-xs text-gray-500 mt-1">
                Required for AI parsing features.
              </p>
            </div>
            <div class="flex justify-end space-x-3 pt-4">
              <button
                on:click={() => (showSettings = false)}
                class="px-4 py-2 rounded-lg text-gray-400 hover:text-white hover:bg-gray-700 transition-colors"
              >
                Cancel
              </button>
              <button
                on:click={saveSettings}
                class="px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white font-medium shadow-lg shadow-blue-500/30 transition-all"
              >
                Save Settings
              </button>
            </div>
          </div>
        </div>
      </div>
    {/if}

    <!-- Main Content -->
    <div
      class="flex-1 min-h-0 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8"
    >
      <!-- Left Panel: Controls -->
      <div class="lg:col-span-1 space-y-6 overflow-y-auto pr-2">
        <!-- Step 1: Select Folder -->
        <div
          class="bg-gray-800/50 rounded-xl p-6 border border-gray-700 backdrop-blur-sm"
        >
          <h2
            class="text-lg font-semibold text-gray-200 mb-4 flex items-center"
          >
            <span
              class="bg-blue-500/20 text-blue-400 w-6 h-6 rounded-full flex items-center justify-center text-xs mr-2"
              >1</span
            >
            Select Source
          </h2>
          <button
            on:click={selectFolder}
            disabled={isLoading}
            class="w-full py-3 px-4 bg-gray-700 hover:bg-gray-600 text-white rounded-lg font-medium transition-all flex items-center justify-center space-x-2 disabled:opacity-50 disabled:cursor-not-allowed group"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="h-5 w-5 text-gray-400 group-hover:text-white transition-colors"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"
              />
            </svg>
            <span>Browse Folder</span>
          </button>
          {#if folderPath}
            <div
              class="mt-3 p-3 bg-gray-900/50 rounded-lg border border-gray-700/50"
            >
              <p
                class="text-xs text-gray-500 uppercase tracking-wider font-bold mb-1"
              >
                Selected Path
              </p>
              <p class="text-sm text-gray-300 break-all font-mono">
                {folderPath}
              </p>
            </div>
          {/if}
        </div>

        <!-- Step 2: Actions -->
        {#if localTracks.length > 0}
          <div
            class="bg-gray-800/50 rounded-xl p-6 border border-gray-700 backdrop-blur-sm animate-fade-in"
          >
            <h2
              class="text-lg font-semibold text-gray-200 mb-4 flex items-center"
            >
              <span
                class="bg-blue-500/20 text-blue-400 w-6 h-6 rounded-full flex items-center justify-center text-xs mr-2"
                >2</span
              >
              Choose Method
            </h2>

            <div class="space-y-4">
              <!-- Template -->
              <!-- Template -->
              <div class="bg-gray-700 rounded-lg p-4 space-y-3">
                <button
                  on:click={generateFromTemplate}
                  disabled={isLoading}
                  class="w-full flex items-center justify-between group disabled:opacity-50"
                >
                  <div class="flex items-center text-white font-medium">
                    <svg
                      xmlns="http://www.w3.org/2000/svg"
                      class="h-5 w-5 mr-3 text-purple-400"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"
                      />
                    </svg>
                    <span>Template Pattern</span>
                  </div>
                  <span class="text-xs text-gray-500 group-hover:text-gray-300"
                    >Fast</span
                  >
                </button>

                <div class="pt-2 border-t border-gray-600">
                  <label class="text-xs text-gray-400 block mb-1">Format</label>
                  <select
                    bind:value={templateFormat}
                    class="w-full bg-gray-800 border border-gray-600 rounded px-2 py-1 text-sm text-white focus:ring-1 focus:ring-blue-500 outline-none"
                  >
                    <option value="Track. Artist - Title"
                      >Track. Artist - Title</option
                    >
                    <option value="Track. Title">Track. Title</option>
                  </select>
                </div>
              </div>

              <!-- AI Parsing -->
              <button
                on:click={parseWithAI}
                disabled={isLoading}
                class="w-full py-3 px-4 bg-gradient-to-r from-indigo-900/50 to-purple-900/50 hover:from-indigo-800/50 hover:to-purple-800/50 border border-indigo-500/30 text-white rounded-lg font-medium transition-all text-left flex items-center justify-between group disabled:opacity-50"
              >
                <div class="flex items-center">
                  <svg
                    xmlns="http://www.w3.org/2000/svg"
                    class="h-5 w-5 mr-3 text-indigo-400"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M13 10V3L4 14h7v7l9-11h-7z"
                    />
                  </svg>
                  <span>AI Smart Parse</span>
                </div>
                <span
                  class="text-xs text-indigo-300 bg-indigo-500/20 px-2 py-0.5 rounded"
                  >New</span
                >
              </button>

              <!-- Bandcamp -->
              <div class="pt-2 border-t border-gray-700">
                <label class="text-xs text-gray-400 font-medium mb-2 block"
                  >Bandcamp Match</label
                >
                <div class="flex space-x-2">
                  <input
                    type="text"
                    bind:value={bandcampUrl}
                    placeholder="Album URL..."
                    disabled={isLoading}
                    class="flex-1 bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm text-white focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none"
                  />
                  <button
                    on:click={fetchAndMatch}
                    disabled={isLoading || !bandcampUrl}
                    class="p-2 bg-blue-600 hover:bg-blue-500 text-white rounded-lg disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                  >
                    <svg
                      xmlns="http://www.w3.org/2000/svg"
                      class="h-5 w-5"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
                      />
                    </svg>
                  </button>
                </div>
              </div>
            </div>
          </div>
        {/if}
      </div>

      <!-- Right Panel: Results -->
      <div class="lg:col-span-2 flex flex-col h-full min-h-0">
        {#if notification}
          <div
            class="mb-4 p-4 rounded-lg bg-gray-800 border-l-4 border-blue-500 shadow-lg animate-fade-in flex items-center flex-none"
          >
            {#if isLoading}
              <svg
                class="animate-spin -ml-1 mr-3 h-5 w-5 text-blue-500"
                xmlns="http://www.w3.org/2000/svg"
                fill="none"
                viewBox="0 0 24 24"
              >
                <circle
                  class="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  stroke-width="4"
                ></circle>
                <path
                  class="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                ></path>
              </svg>
            {/if}
            <span class="text-gray-200">{notification}</span>
          </div>
        {/if}

        {#if processedTracks.length > 0}
          <div
            class="bg-gray-800/50 rounded-xl border border-gray-700 backdrop-blur-sm flex-1 flex flex-col overflow-hidden animate-fade-in min-h-0"
          >
            <div
              class="p-4 border-b border-gray-700 flex justify-between items-center bg-gray-800/80 flex-none"
            >
              <h2 class="font-semibold text-gray-200">Proposed Changes</h2>
              <span class="text-sm text-gray-400"
                >{processedTracks.length} tracks</span
              >
            </div>

            <div class="overflow-y-auto flex-1 p-4 space-y-3">
              {#each processedTracks as match, i}
                <div
                  class="group bg-gray-900/50 rounded-lg p-3 border border-gray-700 hover:border-gray-600 transition-all"
                >
                  <div class="flex items-start space-x-4">
                    <!-- Status Icon -->
                    <div class="mt-1">
                      {#if match.status === "Original"}
                        <div class="w-2 h-2 rounded-full bg-gray-500"></div>
                      {:else}
                        <div
                          class={`w-2 h-2 rounded-full ${match.confidence > 0.8 ? "bg-green-500" : match.confidence > 0.4 ? "bg-yellow-500" : "bg-red-500"}`}
                        ></div>
                      {/if}
                    </div>

                    <div class="flex-1 space-y-2">
                      <!-- Original Name -->
                      <div
                        class="text-xs text-gray-500 font-mono truncate"
                        title={match.originalName}
                      >
                        {match.originalName}
                      </div>

                      <!-- New Name Input -->
                      <div class="relative">
                        <input
                          type="text"
                          value={match.proposedNewName}
                          on:input={(e) => handleProposedNameChange(e, i)}
                          class={`w-full bg-gray-800 border rounded px-3 py-2 text-sm text-white focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none font-mono transition-colors ${getConfidenceColor(match.confidence)}`}
                        />
                      </div>
                    </div>

                    <!-- Status Badge -->
                    <div class="flex flex-col items-end space-y-1 min-w-[80px]">
                      <span
                        class={`text-[10px] uppercase tracking-wider font-bold px-2 py-0.5 rounded-full border ${getConfidenceColor(match.confidence)}`}
                      >
                        {match.status}
                      </span>
                      {#if match.status !== "Original"}
                        <span class="text-xs text-gray-500"
                          >{(match.confidence * 100).toFixed(0)}% match</span
                        >
                      {/if}
                    </div>
                  </div>
                </div>
              {/each}
            </div>

            <div
              class="p-4 border-t border-gray-700 bg-gray-800/80 backdrop-blur-sm flex-none"
            >
              <button
                on:click={renameFiles}
                disabled={isLoading}
                class="w-full py-3 bg-green-600 hover:bg-green-500 text-white rounded-lg font-bold shadow-lg shadow-green-500/20 transition-all disabled:opacity-50 disabled:cursor-not-allowed flex justify-center items-center space-x-2"
              >
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  class="h-5 w-5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M5 13l4 4L19 7"
                  />
                </svg>
                <span>Apply Rename to {processedTracks.length} Files</span>
              </button>
            </div>
          </div>
        {:else if !isLoading && folderPath}
          <div
            class="flex-1 flex items-center justify-center text-gray-500 border-2 border-dashed border-gray-700 rounded-xl m-4"
          >
            <div class="text-center">
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-12 w-12 mx-auto mb-3 opacity-50"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"
                />
              </svg>
              <p>Select a method to generate new names</p>
            </div>
          </div>
        {/if}
      </div>
    </div>
  </div>
</main>

<style>
  /* Custom scrollbar for webkit browsers */
  ::-webkit-scrollbar {
    width: 8px;
    height: 8px;
  }
  ::-webkit-scrollbar-track {
    background: #1f2937;
  }
  ::-webkit-scrollbar-thumb {
    background: #374151;
    border-radius: 4px;
  }
  ::-webkit-scrollbar-thumb:hover {
    background: #4b5563;
  }

  @keyframes fade-in {
    from {
      opacity: 0;
      transform: translateY(10px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
  .animate-fade-in {
    animation: fade-in 0.3s ease-out forwards;
  }
</style>
